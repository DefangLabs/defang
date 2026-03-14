package auth

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"text/template"
	"time"

	"github.com/DefangLabs/defang/src/pkg/term"
)

const successPageTemplate = `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{.Title}}</title>
    <meta name="description" content="Welcome to Defang: Develop Once, Deploy Anywhere." />
  </head>
  <body style="margin:0;font-family:system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;">
    <div style="min-height:100vh;display:flex;align-items:center;justify-content:center;background:linear-gradient(to bottom right, #0f172a, #1e293b);padding:1rem;">
      <div style=" width: 100%; max-width: 420px; background: white; border-radius: 12px; box-shadow: 0 4px 20px rgba(0,0,0,0.15); border: 1px solid #e5e7eb; " >
        <div style="padding: 24px; text-align: center;">
          <div style=" margin: 0 auto 16px auto; height: 64px; width: 64px; display: flex; align-items: center; justify-content: center; border-radius: 9999px; background: #dcfce7; " >
            <svg xmlns="http://www.w3.org/2000/svg" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#16a34a" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="10"></circle>
              <path d="m9 12 2 2 4-4"></path>
            </svg>
          </div>
          <h1 style="margin: 0 0 8px 0; font-size: 22px;">
            {{.Title}}
          </h1>
          <p style="margin: 0; font-size: 14px; color: #6b7280;">
            {{.SuccessMessage}}
          </p>
        </div>
        <div style="padding: 0 24px 24px 24px;">
          <p style="font-size: 14px; color: #6b7280; text-align: center;">
            You can close this window and return to your terminal,
            or explore the Portal below.
          </p>
        </div>
      </div>
    </div>
  </body>
</html>
`

type WaitForOAuthCodeInput struct {
	CallbackPath   string
	Prompt         string
	Title          string
	SuccessMessage string
	BuildAuthURL   func(redirectURL, state string) string
}

// WaitForOAuthCode starts a local HTTP server on a random port to receive an OAuth
// authorization code via redirect callback. It generates a random CSRF state value
// and calls BuildAuthURL with the redirect URL and state to construct the full
// authorization URL. The Prompt is printed to the terminal before opening the browser.
// Returns the authorization code and the redirect URL used for this flow.
func WaitForOAuthCode(ctx context.Context, input WaitForOAuthCodeInput) (code, redirectURL string, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", fmt.Errorf("failed to start local callback server: %w", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port //nolint:forcetypeassert
	redirectURL = fmt.Sprintf("http://127.0.0.1:%d%s", port, input.CallbackPath)
	state := rand.Text()[:16]

	authURL := input.BuildAuthURL(redirectURL, state)

	term.Println(input.Prompt)
	term.Printf("  %s\n", authURL)
	ctx, done := term.OpenBrowserOnEnter(ctx, authURL)
	defer done()

	successPage, err := template.New("success").Parse(successPageTemplate)
	if err != nil {
		return "", "", fmt.Errorf("parsing success page template: %w", err)
	}

	codeCh := make(chan string, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("state") != state {
				http.Error(w, "invalid state", http.StatusBadRequest)
				return
			}
			c := q.Get("code")
			if c == "" {
				http.Error(w, "missing authorization code", http.StatusBadRequest)
				return
			}
			successPage.Execute(w, input) //nolint:errcheck
			codeCh <- c
		}),
	}
	go srv.Serve(ln)  //nolint:errcheck
	defer srv.Close() // No graceful shutdown needed since the server will only handle one request and then be done

	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case code = <-codeCh:
		return code, redirectURL, nil
	}
}
