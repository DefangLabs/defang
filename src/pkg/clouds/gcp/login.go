package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"

	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/term"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

var (
	clientID     = "807925246520-aeouiqtvpde5i6ka1fpj4lh5dccfcga9.apps.googleusercontent.com" // nolint:gosec,G101 // Client ID for app is not treated as secret
	clientSecret = "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"                                      // nolint:gosec,G101 // Client secret for app is not a secret, desktop APP client secrets is considered public information
	scopes       = []string{"email", "https://www.googleapis.com/auth/cloud-platform"}
)

func (gcp *Gcp) Authenticate(ctx context.Context, interactive bool) error {
	// TODO: Add all required permissions for running gcp byoc
	requiredPerms := []string{
		"serviceusage.services.enable",
		"storage.buckets.create",
		"iam.serviceAccounts.create",
		"cloudbuild.builds.create",
	}

	// 1. Try the default application credentials or from the "GOOGLE_APPLICATION_CREDENTIALS" env var if set
	//    - if the user has login with glcoud cli with application default credentials
	//    - if the user has set GOOGLE_APPLICATION_CREDENTIALS to a service account key file with required permissions
	//    - if "GOOGLE_WORKLOAD_IDENTITY_PROVIDER" was set and a credential.json was created for the provider using github token in pkg/login/login.go
	term.Debugf("checking if application default credentials are available and has permission, GOOGLE_APPLICATION_CREDENTIALS=%q...", os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
	if err := testTokenProjectPermissions(ctx, gcp.ProjectId, requiredPerms, nil); err == nil {
		term.Debug("found valid application default credentials with required permissions")
		// No need to pass down ADC token source via options since ADC is automatically used by gcp sdk
		return nil
	}

	// 2. Try load previously saved tokens from the token store
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}
	oauthTokenNames, err := gcp.TokenStore.List("")
	if err != nil {
		return fmt.Errorf("failed to list tokens: %w", err)
	}

	for _, name := range oauthTokenNames {
		tokenJson, err := gcp.TokenStore.Load(name)
		if err != nil {
			return fmt.Errorf("failed to load token %q: %w", name, err)
		}
		var token oauth2.Token
		if err = json.Unmarshal([]byte(tokenJson), &token); err != nil {
			return fmt.Errorf("failed to unmarshal token %q: %w", name, err)
		}
		term.Debugf("Testing token %q from store for required permissions...", name)
		tokenSource := config.TokenSource(ctx, &token)
		if err := testTokenProjectPermissions(ctx, gcp.ProjectId, requiredPerms, tokenSource); err == nil {
			term.Debugf("Token %q is valid and has required permissions\n", name)
			gcp.Options = append(gcp.Options, option.WithTokenSource(tokenSource))
			gcp.TokenSource = tokenSource

			currentToken, err := tokenSource.Token()
			if err != nil {
				return fmt.Errorf("failed to retrieve current token from token source: %w", err)
			}
			if currentToken.AccessToken != token.AccessToken || currentToken.Expiry != token.Expiry || currentToken.RefreshToken != token.RefreshToken {
				term.Debugf("Token %q has been updated, persisting updated token...\n", name)
				bytes, err := json.Marshal(currentToken)
				if err != nil {
					return fmt.Errorf("failed to marshal updated token: %w", err)
				}
				if err := gcp.TokenStore.Save(name, string(bytes)); err != nil {
					return fmt.Errorf("failed to save updated token: %w", err)
				}
			}
			return nil
		} else {
			term.Debugf("Token %q is missing required permissions: %v\n", name, err)
		}
	}

	// 3. If no valid tokens and allow interactive, start interactive login flow
	if !interactive {
		return errors.New("No valid gcloud credentials found") // TODO: Better error message with possible doc link
	}

	term.Debug("no valid tokens found in token store, starting interactive login flow...")
	for range 3 {
		tokenSource, err := gcp.InteractiveLogin(ctx)
		if err != nil {
			return fmt.Errorf("interactive login failed: %w", err)
		}
		if err := testTokenProjectPermissions(ctx, gcp.ProjectId, requiredPerms, tokenSource); err != nil {
			term.Warnf("Token from interactive login is missing required permissions on project %q: %v\n", gcp.ProjectId, err)
			continue
		}
		gcp.Options = append(gcp.Options, option.WithTokenSource(tokenSource))
		gcp.TokenSource = tokenSource
		currentToken, err := tokenSource.Token()
		if err != nil {
			return fmt.Errorf("failed to retrieve current token from token source: %w", err)
		}
		tokenName, err := getEmailFromToken(ctx, currentToken.AccessToken)
		if err != nil {
			return fmt.Errorf("failed to get email from token: %w", err)
		}
		bytes, err := json.Marshal(currentToken)
		if err != nil {
			return fmt.Errorf("failed to marshal token: %w", err)
		}
		if err := gcp.TokenStore.Save(tokenName, string(bytes)); err != nil {
			return fmt.Errorf("failed to save token: %w", err)
		}
		return nil
	}
	return errors.New("too many failed GCP login attempts")
}

func (gcp *Gcp) InteractiveLogin(ctx context.Context) (oauth2.TokenSource, error) {
	pkce, err := auth.GeneratePKCE(64, auth.S256Method)
	if err != nil {
		return nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	cfg := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}

	code, _, err := auth.WaitForOAuthCode(ctx, auth.WaitForOAuthCodeInput{
		CallbackPath:   "/",
		Prompt:         "Please visit the following URL to log in to Google Cloud Platform: (Right click the URL or press ENTER to open browser)",
		Title:          "Logged in to Google Cloud Platform",
		SuccessMessage: "You have successfully logged in to Google Cloud Platform.",
		BuildAuthURL: func(redirectURL, state string) string {
			cfg.RedirectURL = redirectURL
			return cfg.AuthCodeURL(state,
				oauth2.AccessTypeOffline,
				oauth2.SetAuthURLParam("code_challenge", pkce.Challenge),
				oauth2.SetAuthURLParam("code_challenge_method", "S256"),
			)
		},
	})
	if err != nil {
		return nil, err
	}

	token, err := cfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pkce.Verifier))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}

	return cfg.TokenSource(ctx, token), nil
}

func testTokenProjectPermissions(ctx context.Context, projectID string, perms []string, tokenSource oauth2.TokenSource) error {
	var options []option.ClientOption
	if tokenSource != nil {
		options = append(options, option.WithTokenSource(tokenSource))
	}
	client, err := resourcemanager.NewProjectsClient(ctx, options...)
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	defer client.Close()

	req := &iampb.TestIamPermissionsRequest{
		Resource:    "projects/" + projectID,
		Permissions: perms,
	}

	resp, err := client.TestIamPermissions(ctx, req)
	if err != nil {
		return fmt.Errorf("API call failed: %w", err)
	}

	missingPerms := []string{}
	for _, p := range perms {
		if !slices.Contains(resp.Permissions, p) {
			missingPerms = append(missingPerms, p)
		}
	}
	if len(missingPerms) > 0 {
		return fmt.Errorf("token is missing required permissions: %v", missingPerms)
	}

	return nil
}
