package login

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/github"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type LoginFlow = auth.LoginFlow

// Example credentials.json for workload identity federation with GitHub Actions:
//
//	{
//	 "universe_domain": "googleapis.com",
//	 "type": "external_account",
//	 "audience": "//iam.googleapis.com/projects/979169844604/locations/global/workloadIdentityPools/defang-github/providers/github-actions-mu4q9u",
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
//	  "client_id": "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com", // FIXED client id for gcloud cli
//	  "client_secret": "d-FL95Q19q7MQmFpd7hHD0Ty", // Fixed client secret for gcloud cli, not a secret
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

type AuthService interface {
	login(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) (string, error)
}

type OpenAuthService struct{}

func (OpenAuthService) login(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) (string, error) {
	term.Debug("Logging in to", fabricAddr)

	code, err := auth.StartAuthCodeFlow(ctx, flow, func(token string) {
		client.SaveAccessToken(fabricAddr, token)
	}, mcpClient)
	if err != nil {
		return "", err
	}

	return auth.ExchangeCodeForToken(ctx, code) // no scopes = unrestricted
}

var authService AuthService = OpenAuthService{}

func InteractiveLogin(ctx context.Context, fabricAddr string) error {
	return interactiveLogin(ctx, fabricAddr, auth.CliFlow, "CLI-Flow")
}

func InteractiveLoginMCP(ctx context.Context, fabricAddr string, mcpClient string) error {
	return interactiveLogin(ctx, fabricAddr, auth.McpFlow, mcpClient)
}

func interactiveLogin(ctx context.Context, fabricAddr string, flow LoginFlow, mcpClient string) error {
	token, err := authService.login(ctx, fabricAddr, flow, mcpClient)
	if err != nil {
		return err
	}

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	if err := client.SaveAccessToken(fabricAddr, token); err != nil {
		term.Warn(err)
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			term.Printf("\nTo fix file permissions, run:\n\n  sudo chown -R $(whoami) %q\n", pathError.Path)
		}
		// We continue even if we can't save the token; we just won't have it saved for next time
	}
	// The new login page shows the ToS so a successful login implies the user agreed
	return nil
}

func NonInteractiveGitHubLogin(ctx context.Context, fabric client.FabricClient, fabricAddr string) error {
	term.Debug("Non-interactive login using GitHub Actions id-token")
	idToken, err := github.GetIdToken(ctx, "") // default audience (ie. https://github.com/ORG)
	if err != nil {
		return fmt.Errorf("non-interactive login failed: %w", err)
	}
	term.Debug("Got GitHub Actions id-token")

	// Create a Fabric token using the GitHub token as an assertion
	resp, err := fabric.Token(ctx, &defangv1.TokenRequest{
		Assertion: idToken,
		Scope:     []string{"admin", "read", "delete", "tail"},
	})
	if err != nil {
		return err
	}

	// Create the state folder and write the token to a file
	if err := client.SaveAccessToken(fabricAddr, resp.AccessToken); err != nil {
		return err
	}

	// If AWS_ROLE_ARN is set, we're doing "Assume Role with Web Identity"
	if os.Getenv("AWS_ROLE_ARN") != "" && os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE") == "" {
		// AWS_ROLE_ARN is set, but AWS_WEB_IDENTITY_TOKEN_FILE is empty: write the token to a new file
		jwtPath, err := writeWebIdentityToken(fabricAddr, resp.AccessToken)
		if err != nil {
			return err
		}
		// Set AWS env vars for this CLI invocation; future invocations are handled by client.GetExistingToken
		os.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", jwtPath)
		os.Setenv("AWS_ROLE_SESSION_NAME", "defang-cli") // TODO: from WhoAmI
	} else {
		term.Debugf("AWS_WEB_IDENTITY_TOKEN_FILE is already set; not writing token to a new file")
	}

	// If both ACTIONS_ID_TOKEN_REQUEST_URL and GOOGLE_WORKLOAD_IDENTITY_PROVIDER are set, we're doing "Workload Identity Federation" with GCP using github id token
	githubTokenReqUrl := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL")
	gcpProvider := os.Getenv("GOOGLE_WORKLOAD_IDENTITY_PROVIDER")
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
		jwtPath, err := writeWebIdentityToken(fabricAddr+"-gcp", gcpIdToken)
		if err != nil {
			return err
		}

		// 3. Create a cred.json to be used as the GOOGLE_APPLICATION_CREDENTIALS for GCP authentication
		credentials := GoogleAuthCredentials{
			UniverseDomain:   "googleapis.com",
			Type:             "external_account",
			Audience:         audience,
			SubjectTokenType: "urn:ietf:params:oauth:token-type:jwt",
			TokenURL:         "https://sts.googleapis.com/v1/token",
			CredentialSource: &GoogleAuthCredentialSource{
				File: jwtPath,
				Format: &GoogleAuthCredentialFormat{
					Type: "text", // type text for encoded jwt
				},
			},
		}
		credsPath, err := writeCredentialsFile(fabricAddr, credentials)
		if err != nil {
			return err
		}
		// Not an official env var, but our GCP integration will look for this when the provider is set to GCP and this env var is present
		os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", credsPath)
	}

	return err
}

func writeWebIdentityToken(cluster, token string) (string, error) {
	jwtPath, _ := client.GetWebIdentityTokenFile(cluster)
	term.Debugf("writing web identity token to %s", jwtPath)
	if err := os.WriteFile(jwtPath, []byte(token), 0600); err != nil {
		return "", fmt.Errorf("failed to save web identity token: %w", err)
	}
	return jwtPath, nil
}

func writeCredentialsFile(cluster string, creds GoogleAuthCredentials) (string, error) {
	credsBytes, err := json.Marshal(creds)
	if err != nil {
		return "", fmt.Errorf("failed to marshal credentials: %w", err)
	}

	credsPath := path.Join(client.StateDir, client.TokenStorageName(cluster+"-gcp-creds")) + ".json"
	term.Debugf("writing credentials file to %s", credsPath)
	if err := os.WriteFile(credsPath, credsBytes, 0600); err != nil {
		return "", fmt.Errorf("failed to save credentials file: %w", err)
	}
	return credsPath, nil
}

// InteractiveRequireLoginAndToS ensures the user is logged in and has agreed to the terms of service.
// If necessary, it will reconnect to the server as the right tenant, returning the updated Fabric client.
func InteractiveRequireLoginAndToS(ctx context.Context, fabric client.FabricClient, fabricAddr string) (client.FabricClient, error) {
	var err error
	if err = fabric.CheckLoginAndToS(ctx); err != nil {
		// Login interactively now; only do this for authorization-related errors
		if connect.CodeOf(err) == connect.CodeUnauthenticated {
			term.Debug("Server error:", err)
			term.Warn("Please log in to continue.")
			term.ResetWarnings() // clear any previous warnings so we don't show them again

			defer func() { track.Cmd(nil, "Login", P("reason", err)) }()
			if err = InteractiveLogin(ctx, fabricAddr); err != nil {
				return fabric, err
			}

			// Reconnect with the new token
			if newFabric, err := cli.ConnectWithTenant(ctx, fabricAddr, fabric.GetRequestedTenant()); err != nil {
				return fabric, err
			} else {
				fabric = newFabric
				track.Tracker = fabric // update global tracker
			}

			if err = fabric.CheckLoginAndToS(ctx); err == nil { // recheck (new token = new user)
				return fabric, nil // success
			}
		}

		// Check if the user has agreed to the terms of service and show a prompt if needed
		if connect.CodeOf(err) == connect.CodeFailedPrecondition {
			term.Warn(client.PrettyError(err))

			defer func() { track.Cmd(nil, "Terms", P("reason", err)) }()
			if err = InteractiveAgreeToS(ctx, fabric); err != nil {
				return fabric, err // fatal
			}
		}
	}
	return fabric, err
}
