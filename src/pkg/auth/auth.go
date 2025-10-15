package auth

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/pkg/browser"
)

var openAuthClient = NewClient("defang-cli", pkg.Getenv("DEFANG_ISSUER", "https://auth.defang.io"))

const (
	authTemplateString = `<!DOCTYPE html>
<html>
	<head>
		<title>Defang | Authentication Status</title>
		<style>
			body {
				font-family: 'Exo', sans-serif;
				background: linear-gradient(to right, #1e3c72, #2a5298);
				color: white;
				display: flex;
				justify-content: center;
				align-items: center;
				height: 100vh;
				margin: 0;
			}
			.container {
				text-align: center;
			}
			.status-message {
				font-size: 2em;
				margin-bottom: 1em;
			}
			.close-link {
				font-size: 1em;
				cursor: pointer;
			}
		</style>
	</head>
	<body>
		<div class="container">
			<h1>Welcome to Defang</h1>
			<p class="status-message">{{.StatusMessage}}</p>
			<p class="close-link" onclick="window.close()">You can close this window.</p>
		</div>
	</body>
</html>`
)

var (
	authTemplate = template.Must(template.New("auth").Parse(authTemplateString))
)

type AuthCodeFlow struct {
	code        string
	redirectUri string
	verifier    string
}

type LoginFlow bool

const (
	CliFlow LoginFlow = false
	McpFlow LoginFlow = true
)

// buildRedirectUri constructs the appropriate redirect URI based on the environment
func buildRedirectUri(authPort int) string {
	// Check if we're running in GitHub Codespaces
	if codespaceURL := pkg.Getenv("CODESPACE_NAME", ""); codespaceURL != "" {
		// In Codespaces, construct the public URL
		// Format: https://{codespace-name}-{port}.app.github.dev
		redirectUri := fmt.Sprintf("https://%s-%d.app.github.dev/auth", codespaceURL, authPort)
		term.Debug("Detected GitHub Codespaces environment, using redirect URI:", redirectUri)
		return redirectUri
	}

	// Default to localhost for local development
	redirectUri := fmt.Sprintf("http://127.0.0.1:%d/auth", authPort)
	term.Debug("Using local development redirect URI:", redirectUri)
	return redirectUri
}

// TODO: make the server stop once we have the code
func ServeAuthCodeFlowServer(ctx context.Context, authPort int, tenant types.TenantName, saveToken func(string)) error {
	// Detect if we're running in GitHub Codespaces
	redirectUri := buildRedirectUri(authPort)

	// Get the authorization URL before setting up the handler
	ar, err := openAuthClient.Authorize(redirectUri, CodeResponseType, WithPkce())
	if err != nil {
		return err
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			slog.Info("redirecting to " + ar.url.String())
			http.Redirect(w, r, ar.url.String(), http.StatusFound)
			return
		}
		if r.URL.Path != "/auth" {
			http.NotFound(w, r)
			return
		}

		// Handle forwarded headers (for reverse proxies like Codespaces)
		if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
			r.URL.Scheme = scheme
		}

		if host := r.Header.Get("X-Forwarded-Host"); host != "" {
			r.Host = host
			r.URL.Host = host
		}

		var msg string
		query := r.URL.Query()
		switch {
		case query.Get("error") != "":
			msg = "Authentication failed: " + query.Get("error_description")
			slog.Error("authentication failed", "error", query.Get("error_description"))
		case query.Get("state") != ar.state:
			msg = "Authentication error: state mismatch"
			slog.Error("authentication error: state mismatch", "state", query.Get("state"), "expected", ar.state)
		default:
			msg = "Authentication successful"
			token, err := ExchangeCodeForToken(ctx, AuthCodeFlow{code: query.Get("code"), redirectUri: redirectUri, verifier: ar.verifier}, tenant, 0)
			if err != nil {
				slog.Error("failed to exchange code for token", "error", err)
				msg = "Authentication failed: " + err.Error()
			}
			saveToken(token)
		}
		authTemplate.Execute(w, struct{ StatusMessage string }{msg})
	})

	// set the port and the handler to the server
	server := &http.Server{
		Addr:              "0.0.0.0:" + strconv.Itoa(authPort),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Start the server
	err = server.ListenAndServe()
	if err != nil {
		return err
	}

	return nil
}

func StartAuthCodeFlow(ctx context.Context, mcpFlow LoginFlow) (AuthCodeFlow, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	redirectUri := openAuthClient.GetPollRedirectURI()

	opts := []AuthorizeOption{
		WithPkce(),
	}
	ar, err := openAuthClient.Authorize(redirectUri, CodeResponseType, opts...)
	if err != nil {
		return AuthCodeFlow{}, err
	}

	state := ar.state
	authorizeUrl := ar.url.String()
	term.Debug("Authorization URL:", authorizeUrl)

	n, _ := term.Printf("Please visit the authorization URL to log in. (Right click the URL or press ENTER to open browser)\r")
	defer term.Print(strings.Repeat(" ", n), "\r") // TODO: use termenv to clear line

	// TODO:This is used to open the browser for GitHub Auth before blocking
	if mcpFlow {
		err := browser.OpenURL(authorizeUrl)
		if err != nil {
			return AuthCodeFlow{}, fmt.Errorf("failed to open browser: %w", err)
		}
	}

	input := term.NewNonBlockingStdin()
	defer input.Close() // abort the read
	go func() {
		var b [1]byte
		for {
			if _, err := input.Read(b[:]); err != nil {
				return // exit goroutine
			}
			switch b[0] {
			case 3: // Ctrl-C
				cancel()
			case 10, 13: // Enter or Return
				err := browser.OpenURL(authorizeUrl)
				if err != nil {
					term.Errorf("failed to open browser: %v", err)
				}
			default:
			}
		}
	}()

	for {
		code, err := openAuthClient.Poll(ctx, state)
		if err != nil {
			if errors.Is(err, ErrPollTimeout) {
				term.Debug("poll timed out, retrying...")
				continue // just retry
			}
			return AuthCodeFlow{}, err
		}
		return AuthCodeFlow{code: code, redirectUri: redirectUri, verifier: ar.verifier}, nil
	}
}

func ExchangeCodeForToken(ctx context.Context, code AuthCodeFlow, tenant types.TenantName, ttl time.Duration, ss ...scope.Scope) (string, error) {
	var scopes []string
	for _, s := range ss {
		if s == scope.Admin {
			scopes = nil
			break
		}
		scopes = append(scopes, s.String())
	}

	term.Debugf("Generating token for tenant %q with scopes %v", tenant, scopes)

	token, err := openAuthClient.Exchange(code.code, code.redirectUri, code.verifier) // TODO: scopes, TTL
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
