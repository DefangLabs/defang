package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"cloud.google.com/go/iam/apiv1/iampb"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
)

var (
	clientID     = "807925246520-aeouiqtvpde5i6ka1fpj4lh5dccfcga9.apps.googleusercontent.com" // nolint:gosec,G101 // Client ID for app is not treated as secret
	clientSecret = "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"                                      // nolint:gosec,G101 // Client secret for app is not a secret, desktop APP client secrets is considered public information
	scopes       = []string{"email", "https://www.googleapis.com/auth/cloud-platform"}

	// TODO: Add all required permissions for running gcp byoc
	requiredPerms = []string{
		"serviceusage.services.enable",
		"storage.buckets.create",
		"iam.serviceAccounts.create",
		"cloudbuild.builds.create",
	}
)

// Example credentials.json for workload identity federation with GitHub Actions:
//
//	{
//	 "universe_domain": "googleapis.com",
//	 "type": "external_account",
//	 "audience": "//iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/defang-github/providers/github-actions-mu4q9u",
//	 "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
//	 "token_url": "https://sts.googleapis.com/v1/token",
//	 "credential_source": {
//	   "file": "/home/edw/defang/tmp/gcptokencred/token.jwt",
//	   "format": {
//	     "type": "text"
//	   }
//	 }
//	}
//
// Example credentials.json for user credentials from `gcloud auth application-default login`:
//
//	{
//	  "account": "",
//	  "client_id": "123456789012-CLIENTID.apps.googleusercontent.com", // FIXED client id for gcloud cli
//	  "client_secret": "d-CLIENTSECRET", // Fixed client secret for gcloud cli, not a secret
//	  "quota_project_id": "test-quota-project-id",
//	  "refresh_token": "1//XXXXXXXXXXXXXXXXXXXXXXXXXXXX-YYYYYY-BZjZCABhoInO4zaMg6DhZzl2gMbr273cB5Mo1nBSNL5FjntKhUaMJW2IFnKAZmZE",
//	  "type": "authorized_user",
//	  "universe_domain": "googleapis.com"
//	}
type GoogleAuthCredentials struct {
	Account          string                      `json:"account,omitempty"`
	ClientID         string                      `json:"client_id,omitempty"`
	ClientSecret     string                      `json:"client_secret,omitempty"`
	QuotaProjectID   string                      `json:"quota_project_id,omitempty"`
	RefreshToken     string                      `json:"refresh_token,omitempty"`
	UniverseDomain   string                      `json:"universe_domain,omitempty"`
	Type             string                      `json:"type,omitempty"`
	Audience         string                      `json:"audience,omitempty"`
	SubjectTokenType string                      `json:"subject_token_type,omitempty"`
	TokenURL         string                      `json:"token_url,omitempty"`
	CredentialSource *GoogleAuthCredentialSource `json:"credential_source,omitempty"`
}

type GoogleAuthCredentialSource struct {
	File   string                      `json:"file,omitempty"`
	Format *GoogleAuthCredentialFormat `json:"format,omitempty"`
}

type GoogleAuthCredentialFormat struct {
	Type string `json:"type,omitempty"`
}

func (gcp *Gcp) Authenticate(ctx context.Context, interactive bool) error {
	if err := setupGithubTokenCredentials(ctx); err != nil {
		term.Warnf("failed to setup github token credentials for workload identity federation: %v", err)
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
	if tokenSource, err := gcp.findStoredCredentials(ctx); err != nil {
		term.Warnf("failed to load stored credentials: %v", err)
	} else if tokenSource != nil {
		term.Debug("found valid stored credentials with required permissions")
		gcp.Options = append(gcp.Options, option.WithTokenSource(tokenSource))
		gcp.TokenSource = tokenSource
		return nil
	}

	// 3. If no valid tokens and allow interactive, start interactive login flow
	if !interactive {
		return errors.New("No valid gcloud credentials found") // TODO: Better error message with possible doc link
	}
	term.Debug("no valid tokens found in token store, starting interactive login flow...")
	return gcp.tryInteraciveLogin(ctx, 3)
}

func (gcp *Gcp) tryInteraciveLogin(ctx context.Context, n int) error {
	for range n {
		tokenSource, err := gcp.InteractiveLogin(ctx)
		if err != nil {
			return fmt.Errorf("interactive login failed: %w", err)
		}
		if err := testTokenProjectPermissions(ctx, gcp.ProjectId, requiredPerms, tokenSource); err != nil {
			if errors.As(err, &ErrorMissingPermissions{}) {
				term.Warnf("Token from interactive login is missing required permissions on project %q: %v\nPlease ensure your user has the following permissions: %v\n", gcp.ProjectId, err, requiredPerms)
			} else {
				term.Warnf("Failed to validate token from interactive login on project %q: %v\n", gcp.ProjectId, err)
			}
			term.Warn("Please try logging in again with an account that has the required permissions.")
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

func (gcp *Gcp) findStoredCredentials(ctx context.Context) (oauth2.TokenSource, error) {
	if gcp.TokenStore == nil {
		return nil, nil
	}
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}
	oauthTokenNames, err := gcp.TokenStore.List("")
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	for _, name := range oauthTokenNames {
		tokenJson, err := gcp.TokenStore.Load(name)
		if err != nil {
			term.Warnf("failed to load previously saved auth token %q: %v", name, err)
			continue
		}
		var token oauth2.Token
		if err = json.Unmarshal([]byte(tokenJson), &token); err != nil {
			term.Warnf("failed to parse previously saved auth token %q: %v", name, err)
			continue
		}
		term.Debugf("Testing token %q from store for required permissions...", name)
		tokenSource := config.TokenSource(ctx, &token)
		if err := testTokenProjectPermissions(ctx, gcp.ProjectId, requiredPerms, tokenSource); err == nil {
			term.Debugf("Token %q is valid and has required permissions\n", name)
			currentToken, err := tokenSource.Token()
			if err != nil {
				return nil, fmt.Errorf("failed to retrieve current token from token source: %w", err)
			}
			if currentToken.AccessToken != token.AccessToken || currentToken.Expiry != token.Expiry || currentToken.RefreshToken != token.RefreshToken {
				term.Debugf("Token %q has been updated, persisting updated token...\n", name)
				bytes, err := json.Marshal(currentToken)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal updated token: %w", err)
				}
				if gcp.TokenStore != nil {
					if err := gcp.TokenStore.Save(name, string(bytes)); err != nil {
						return nil, fmt.Errorf("failed to save updated token: %w", err)
					}
				}
			}
			return tokenSource, nil
		} else {
			term.Debugf("Token %q is missing required permissions: %v\n", name, err)
		}
	}
	return nil, nil
}

func setupGithubTokenCredentials(ctx context.Context) error {
	// If both ACTIONS_ID_TOKEN_REQUEST_URL and GOOGLE_WORKLOAD_IDENTITY_PROVIDER are set, we're doing "Workload Identity Federation" with GCP using github id token
	githubTokenReqUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	gcpProvider := os.Getenv("GOOGLE_WORKLOAD_IDENTITY_PROVIDER")
	term.Debugf("ACTIONS_ID_TOKEN_REQUEST_URL=%q, GOOGLE_WORKLOAD_IDENTITY_PROVIDER=%q", githubTokenReqUrl, gcpProvider)
	if githubTokenReqUrl != "" && gcpProvider != "" {
		// 1. convert the identity provider to aud in credentials.json below
		// In credentials.json audience is in the format: //iam.googleapis.com/projects/996411251390/locations/global/workloadIdentityPools/defang-github/providers/github-actions-r6xx29
		audience := gcpProvider
		if !strings.HasPrefix(audience, "//iam.googleapis.com/") {
			audience = "//" + path.Join("iam.googleapis.com", audience)
		}

		// 2. use the aud to get the github id token and save it to a file, which will be referenced in credentials.json as the credential source
		gcpIdToken, err := github.GetIdToken(ctx, audience) // WIF provider requires the jwt audience to match the provider resource name
		if err != nil {
			return fmt.Errorf("non-interactive login failed: %w", err)
		}
		project, pool, provider, err := parseWIFProvider(audience)
		if err != nil {
			return fmt.Errorf("failed to parse WIF provider %q: %w", audience, err)
		}
		tokenKey := fmt.Sprintf("%s-%s-%s.jwt", project, pool, provider)
		jwtPath := filepath.Join(client.StateDir, tokenKey)
		if err := os.WriteFile(jwtPath, []byte(gcpIdToken), 0600); err != nil {
			return fmt.Errorf("failed to save web identity token for gcp: %w", err)
		}

		// 3. Create a credentials.json to be used as the GOOGLE_APPLICATION_CREDENTIALS for GCP authentication
		credentials := GoogleAuthCredentials{
			UniverseDomain:   "googleapis.com",
			Type:             "external_account",
			Audience:         audience,
			SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
			TokenURL:         "https://sts.googleapis.com/v1/token",
			CredentialSource: &GoogleAuthCredentialSource{
				File: jwtPath, // reference the file where we saved the github id token above
				Format: &GoogleAuthCredentialFormat{
					Type: "text", // type text for encoded jwt
				},
			},
		}
		credsPath, err := writeCredentialsFile(tokenKey, credentials)
		if err != nil {
			return err
		}
		// Not an official env var, but our GCP integration will look for this when the provider is set to GCP and this env var is present
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsPath)
	}
	return nil
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

type ErrorMissingPermissions []string

func (e ErrorMissingPermissions) Error() string {
	return fmt.Sprintf("token is missing required permissions: %v", []string(e))
}

var testTokenProjectPermissions = func(ctx context.Context, projectID string, perms []string, tokenSource oauth2.TokenSource) error {
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

	var errMissingPerms ErrorMissingPermissions
	for _, p := range perms {
		if !slices.Contains(resp.Permissions, p) {
			errMissingPerms = append(errMissingPerms, p)
		}
	}
	if len(errMissingPerms) > 0 {
		return errMissingPerms
	}

	return nil
}

func writeCredentialsFile(key string, creds GoogleAuthCredentials) (string, error) {
	credsBytes, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal credentials: %w", err)
	}

	credsPath := filepath.Join(client.StateDir, key+"-gcp-creds") + ".json"
	term.Debugf("writing credentials file to %s", credsPath)
	if err := os.WriteFile(credsPath, credsBytes, 0600); err != nil {
		return "", fmt.Errorf("failed to save credentials file: %w", err)
	}
	return credsPath, nil
}

func parseWIFProvider(s string) (project, pool, provider string, err error) {
	parts := strings.Split(s, "/")

	if len(parts) != 11 ||
		parts[3] != "projects" ||
		parts[5] != "locations" ||
		parts[7] != "workloadIdentityPools" ||
		parts[9] != "providers" {
		return "", "", "", fmt.Errorf("invalid WIF provider string: %q", s)
	}

	return parts[4], parts[8], parts[10], nil
}
