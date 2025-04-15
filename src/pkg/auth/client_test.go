package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pkg/browser"
)

const issuer = "http://localhost:3001" // "https://auth.defang.io"

func TestAuthorize(t *testing.T) {
	client := NewClient("defang-cli", issuer)
	result, err := client.Authorize("http://localhost:1234/", CodeResponseType, WithPkce())
	if err != nil {
		t.Fatalf("Failed to authorize: %v", err)
	}
	if result.state == "" {
		t.Fatal("Expected non-empty state")
	}
	if result.verifier == "" {
		t.Fatal("Expected non-empty verifier")
	}
	challenge := generateChallenge(result.verifier, S256Method)
	expected := issuer + "/authorize?client_id=defang-cli&code_challenge=" + challenge + "&code_challenge_method=S256&redirect_uri=http%3A%2F%2Flocalhost%3A1234%2F&response_type=code&state=" + result.state
	if result.url.String() != expected {
		t.Fatalf("Expected URL %s, got %s", expected, result.url.String())
	}
}

func TestExchange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	redirectCh := make(chan url.URL)
	defer close(redirectCh)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/callback":
			redirectCh <- *r.URL
			http.Redirect(w, r, "/ok", http.StatusFound)
		case "/ok":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	tests := []struct {
		name string
		opts []AuthorizeOption
	}{
		{name: "no pkce"},
		{name: "with pkce", opts: []AuthorizeOption{WithPkce()}},
	}

	client := NewClient("defang-cli", issuer)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectUrl := server.URL + "/callback"
			authorizeResult, err := client.Authorize(redirectUrl, CodeResponseType, tt.opts...)
			if err != nil {
				t.Fatalf("Failed to authorize: %v", err)
			}

			// const xx = "https://auth.defang.io/client?callback=" +
			browser.OpenURL(authorizeResult.url.String())
			// Wait for the redirect to be received
			redirectURL := <-redirectCh

			if authorizeResult.state != redirectURL.Query().Get("state") {
				t.Error("State mismatch between authorize and redirect URL")
			}
			code := redirectURL.Query().Get("code")
			if code == "" {
				t.Error("Expected non-empty code in redirect URL")
			}
			exchangeResult, err := client.Exchange(code, redirectUrl, authorizeResult.verifier)
			if err != nil {
				t.Fatalf("Failed to exchange code: %v", err)
			}
			if exchangeResult.AccessToken == "" {
				t.Fatal("Expected non-empty access token")
			}
			if exchangeResult.RefreshToken == "" {
				t.Fatal("Expected non-empty refresh token")
			}
		})
	}
}
