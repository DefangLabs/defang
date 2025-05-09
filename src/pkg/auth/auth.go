package auth

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
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

func NewAuthCodeFlow(ar *AuthorizeResult, code string) AuthCodeFlow {
	return AuthCodeFlow{
		code:        code,
		redirectUri: ar.url.String(),
		verifier:    ar.verifier,
	}
}

type Prompt bool

const (
	PromptNo  Prompt = false
	PromptYes Prompt = true
)

func StartAuthCodeFlowWithDocker(ctx context.Context, authPort int, ch *chan string, ar *AuthorizeResult) {

	// We want to make the handler for the auth server
	authorizeUrl := ar.url.String()
	term.Debug("Authorization URL:", authorizeUrl)
	state := ar.state
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, authorizeUrl, http.StatusFound)
			return
		}
		if r.URL.Path != "/auth" {
			http.NotFound(w, r)
			return
		}
		var msg string
		query := r.URL.Query()
		switch {
		case query.Get("error") != "":
			msg = "Authentication failed: " + query.Get("error_description")
		case query.Get("state") != state:
			msg = "Authentication error: state mismatch"
		default:
			msg = "Authentication successful"
			*ch <- query.Get("code")
		}
		authTemplate.Execute(w, struct{ StatusMessage string }{msg})
	})

	// set the port and the handler to the server
	server := &http.Server{Addr: "0.0.0.0:" + strconv.Itoa(authPort), Handler: handler}

	// Start the server in a goroutine that will continue forever
	go func() {
		err := server.ListenAndServe()
		if err != nil {
			// Log error but continue
			term.Debugf("HTTP server error: %v", err)
			// If port is in use, the auth flow will fail with context cancellation
		}
	}()

	// We will handle the reader in the caller, which should not block this function since we need it to
	// run forever
}

func StartAuthCodeFlow(ctx context.Context, prompt Prompt) (AuthCodeFlow, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Generate random state
	var state string

	// Create a channel to wait for the server to finish
	ch := make(chan string)
	defer close(ch)

	var authorizeUrl string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, authorizeUrl, http.StatusFound)
			return
		}
		if r.URL.Path != "/auth" {
			http.NotFound(w, r)
			return
		}
		var msg string
		query := r.URL.Query()
		switch {
		case query.Get("error") != "":
			msg = "Authentication failed: " + query.Get("error_description")
		case query.Get("state") != state:
			msg = "Authentication error: state mismatch"
		default:
			msg = "Authentication successful"
			ch <- query.Get("code")
		}
		authTemplate.Execute(w, struct{ StatusMessage string }{msg})
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	redirectUri := server.URL + "/auth"
	ar, err := openAuthClient.Authorize(redirectUri, CodeResponseType, WithPkce(), WithProvider("github"))
	if err != nil {
		return AuthCodeFlow{}, err
	}

	state = ar.state
	authorizeUrl = ar.url.String()
	term.Debug("Authorization URL:", authorizeUrl)

	n, _ := term.Printf("Please visit %s and log in. (Right click the URL or press ENTER to open browser)\r", server.URL)
	defer term.Print(strings.Repeat(" ", n), "\r") // TODO: use termenv to clear line

	// TODO:This is used to open the browser for GitHub Auth before blocking
	if prompt {
		browser.OpenURL(server.URL)
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
				browser.OpenURL(server.URL)
			}
		}
	}()

	select {
	case <-ctx.Done():
		return AuthCodeFlow{}, ctx.Err()
	case code := <-ch:
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
