package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/pkg/browser"
)

var openAuthClient = NewClient("defang-cli", pkg.Getenv("DEFANG_ISSUER", "https://auth.defang.io"))

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

func StartAuthCodeFlow(ctx context.Context, mcpFlow LoginFlow, saveToken func(string), mcpClient string) (AuthCodeFlow, error) {
	ctx, cancel := context.WithCancel(ctx)
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

	term.Println(authorizeUrl)
	n, _ := term.Printf("Please visit the above URL to log in. (Right click the URL or press ENTER to open browser)\r")
	defer term.Print(strings.Repeat(" ", n), "\r") // TODO: use termenv to clear line

	// TODO:This is used to open the browser for GitHub Auth before blocking
	if mcpFlow {
		err := errors.New("no browser found in codespaces")
		if mcpClient != "vscode-codespaces" {
			err = browser.OpenURL(authorizeUrl)
		}
		if err != nil {
			go func() {
				ctx := context.Background()
				code, err := pollForAuthCode(ctx, state)
				if err != nil {
					term.Errorf("failed to poll for auth code: %v", err)
					return
				}

				token, err := ExchangeCodeForToken(ctx, AuthCodeFlow{code: code, redirectUri: redirectUri, verifier: ar.verifier})
				if err != nil {
					term.Errorf("failed to exchange code for token: %v", err)
					return
				}

				saveToken(token)
			}()
			// If we can't open the browser, just print the URL and let the user open it themselves
			return AuthCodeFlow{}, ErrNoBrowser{Err: err, URL: authorizeUrl}
		}
	} else {
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
	}

	code, err := pollForAuthCode(ctx, state)
	if err != nil {
		return AuthCodeFlow{}, err
	}
	return AuthCodeFlow{code: code, redirectUri: redirectUri, verifier: ar.verifier}, nil
}

func pollForAuthCode(ctx context.Context, state string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	for {
		code, err := openAuthClient.Poll(ctx, state)
		if err != nil {
			if errors.Is(err, ErrPollTimeout) {
				term.Debug("poll timed out, retrying...")
				continue // retry
			}
			var unexpectedError ErrUnexpectedStatus
			if errors.As(err, &unexpectedError) && unexpectedError.StatusCode >= 500 {
				term.Debugf("received server error: %s, retrying...", unexpectedError.Status)
				continue // retry
			}
			return "", err
		}

		return code, nil
	}
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

	term.Debugf("Generating access token with scopes %v", scopes)

	token, err := openAuthClient.Exchange(code.code, code.redirectUri, code.verifier) // TODO: scope
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func ExchangeJWTForToken(ctx context.Context, jwt string) (string, error) {
	term.Debugf("Generating token for jwt %q", jwt)

	token, err := openAuthClient.ExchangeJWT(jwt) // TODO: scopes, TTL
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
