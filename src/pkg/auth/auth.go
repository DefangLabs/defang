package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/pkg/browser"
)

var OpenAuthClient = NewClient("defang-cli", pkg.Getenv("DEFANG_ISSUER", "https://auth.defang.io"))

type ErrNoBrowser struct {
	Err error
	URL string
}

func (e ErrNoBrowser) Error() string {
	return fmt.Sprintf("failed to open browser: %v. Please open the following URL in a browser to login: %s", e.Err, e.URL)
}

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

func GetAuthorizeUrl(authType string, parts ...string) string {
	return fmt.Sprintf("%s/%s/%s", OpenAuthClient.issuer, authType, path.Join(parts...))
}

func StartAuthCodeFlow(ctx context.Context, mcpFlow LoginFlow, saveToken func(string), mcpClient string) (AuthCodeFlow, error) {
	redirectUri := OpenAuthClient.GetPollRedirectURI()

	opts := []AuthorizeOption{
		WithPkce(),
	}
	ar, err := OpenAuthClient.Authorize(redirectUri, CodeResponseType, opts...)
	if err != nil {
		return AuthCodeFlow{}, err
	}

	// Create a shortened authorize URL by only including the variable parts (state and code_challenge)
	authorizeUrl := GetAuthorizeUrl("cli", ar.state, ar.challenge)

	fmt.Println("Please visit the following URL to log in: (Right click the URL or press ENTER to open browser)")
	n, _ := term.Printf("  %s", authorizeUrl)
	defer term.Print("\r", strings.Repeat(" ", n), "\r") // TODO: use termenv to clear line

	// TODO:This is used to open the browser for GitHub Auth before blocking
	if mcpFlow {
		err := errors.New("no browser found in codespaces")
		if mcpClient != "vscode-codespaces" {
			err = browser.OpenURL(authorizeUrl)
		}
		if err != nil {
			go func() {
				ctx := context.Background()
				code, err := pollForAuthCode(ctx, ar.state)
				if err != nil {
					slog.ErrorContext(ctx, fmt.Sprintf("failed to poll for auth code: %v", err))
					return
				}

				token, err := ExchangeCodeForToken(ctx, AuthCodeFlow{code: code, redirectUri: redirectUri, verifier: ar.verifier})
				if err != nil {
					slog.ErrorContext(ctx, fmt.Sprintf("failed to exchange code for token: %v", err))
					return
				}

				saveToken(token)
			}()
			// If we can't open the browser, just print the URL and let the user open it themselves
			return AuthCodeFlow{}, ErrNoBrowser{Err: err, URL: authorizeUrl}
		}
	} else {
		var done func()
		ctx, done = term.OpenBrowserOnEnter(ctx, authorizeUrl)
		defer done()
	}

	code, err := pollForAuthCode(ctx, ar.state)
	if err != nil {
		return AuthCodeFlow{}, err
	}
	return AuthCodeFlow{code: code, redirectUri: redirectUri, verifier: ar.verifier}, nil
}

func Poll(ctx context.Context, key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	retryDelay := 500 * time.Millisecond
	const maxRetryDelay = 5 * time.Second

	for {
		result, err := OpenAuthClient.Poll(ctx, key)
		if err != nil {
			if errors.Is(err, ErrPollTimeout) {
				slog.Debug("poll timed out, retrying...")
				continue
			}
			var unexpectedError ErrUnexpectedStatus
			if errors.As(err, &unexpectedError) && unexpectedError.StatusCode >= 500 {
				slog.Debug("received server error, retrying", "status", unexpectedError.Status, "retryDelay", retryDelay)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(retryDelay):
				}
				retryDelay = min(retryDelay*2, maxRetryDelay)
				continue
			}
			return nil, err
		}
		return result, nil
	}
}

func pollForAuthCode(ctx context.Context, state string) (string, error) {
	body, err := Poll(ctx, state)
	if err != nil {
		return "", err
	}
	// Parse the response body as form-urlencoded
	query, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if errorMsg := query.Get("error"); errorMsg != "" {
		return "", fmt.Errorf("authentication failed: %s", query.Get("error_description"))
	}
	code := query.Get("code")
	if code == "" {
		return "", errors.New("no code received from auth server")
	}
	return code, nil
}

func ExchangeCodeForToken(ctx context.Context, code AuthCodeFlow, ss ...scope.Scope) (string, error) {
	var scopes []string
	for _, s := range ss {
		if s == scope.Admin {
			scopes = nil
			break
		}
		scopes = append(scopes, s.String())
	}

	slog.Debug("Generating access token", "scopes", scopes)

	token, err := OpenAuthClient.Exchange(code.code, code.redirectUri, code.verifier) // TODO: scope
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
