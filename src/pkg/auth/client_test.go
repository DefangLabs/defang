package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/pkg/browser"
)

func TestAuthorize(t *testing.T) {
	const issuer = "https://auth.defang.io"

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Write([]byte(`{"error":"invalid_request","error_description":"Invalid request"}`))
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.CloseClientConnections)

	client := NewClient("defang-cli", server.URL)

	t.Run("error json", func(t *testing.T) {
		_, err := client.Exchange("invalid-code", "http://asdf", "")
		const expected = "invalid authorization code: Invalid request"
		if err.Error() != expected {
			t.Fatalf("Expected error %q, got: %v", expected, err)
		}
	})

	t.Run("http error", func(t *testing.T) {
		client.issuer = server.URL + "/404"
		_, err := client.Exchange("invalid-code", "http://asdf", "")
		const expected = `token exchange failed: invalid character 'N' looking for beginning of value: 404 Not Found`
		if err.Error() != expected {
			t.Fatalf("Expected error %q, got: %v", expected, err)
		}
	})
}

func TestExchangeJWT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST method, got %s", r.Method)
			}
			if expected, got := "urn:ietf:params:oauth:grant-type:jwt-bearer", r.PostFormValue("grant_type"); expected != got {
				t.Errorf("Expected grant_type %s, got: %s", expected, got)
			}

			jwt := r.PostFormValue("assertion")
			if jwt == "valid-jwt" {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"access_token":"jwt-access-token","refresh_token":"jwt-refresh-token"}`))
			} else {
				w.Write([]byte(`{"error":"invalid_request","error_description":"Invalid request"}`))
			}
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.CloseClientConnections)

	client := NewClient("defang-cli", server.URL)

	t.Run("success", func(t *testing.T) {
		result, err := client.ExchangeJWT("valid-jwt")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result.AccessToken != "jwt-access-token" {
			t.Errorf("Expected access token 'jwt-access-token', got: %s", result.AccessToken)
		}
		if result.RefreshToken != "jwt-refresh-token" {
			t.Errorf("Expected refresh token 'jwt-refresh-token', got: %s", result.RefreshToken)
		}
	})

	t.Run("invalid jwt", func(t *testing.T) {
		_, err := client.ExchangeJWT("invalid-jwt")
		const expected = "invalid JWT: Invalid request"
		if err.Error() != expected {
			t.Fatalf("Expected error %q, got: %v", expected, err)
		}
	})
}

func TestAuthorizeExchange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping browser test in short mode.")
	}

	redirectCh := make(chan url.URL)
	defer close(redirectCh)

	const redirectPath = "/callback"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			redirectUri := r.URL.Query().Get("redirect_uri")
			http.Redirect(w, r, redirectUri+"?code=1234&state="+r.URL.Query().Get("state"), http.StatusFound)
		case redirectPath:
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<html><script>window.close()</script></html>`))
			redirectCh <- *r.URL
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"access-token","refresh_token":"1234"}`))
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

	client := NewClient("defang-cli", server.URL)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectUrl := server.URL + redirectPath
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
			if exchangeResult.AccessToken != "access-token" {
				t.Error("Expected access token 'access-token'")
			}
			if exchangeResult.RefreshToken != "1234" {
				t.Error("Expected refresh token '1234'")
			}
			// if exchangeResult.ExpiresIn == 0 {
			// 	t.Error("Expected non-zero expires_in")
			// }
		})
	}
}

func TestRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST method, got %s", r.Method)
			}
			if expected, got := "refresh_token", r.PostFormValue("grant_type"); expected != got {
				t.Errorf("Expected grant_type %s, got: %s", expected, got)
			}
			w.Header().Set("Content-Type", "application/json")
			if expected, got := "refresh-token", r.PostFormValue("refresh_token"); expected != got {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"invalid_request","error_description":"Invalid refresh token"}`))
			} else {
				w.Write([]byte(`{"access_token":"access-token2","refresh_token":"12345"}`))
			}
		default:
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient("defang-cli", server.URL)

	t.Run("success", func(t *testing.T) {
		refreshResult, err := client.Refresh("refresh-token")
		if err != nil {
			t.Fatalf("Failed to refresh token: %v", err)
		}
		if refreshResult.AccessToken != "access-token2" {
			t.Error("Expected access token 'access-token2'")
		}
		if refreshResult.RefreshToken != "12345" {
			t.Error("Expected refresh token '12345'")
		}
	})

	t.Run("invalid", func(t *testing.T) {
		refreshResult, err := client.Refresh("invalid-token")
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !errors.Is(err, ErrInvalidRefreshToken) {
			t.Errorf("Expected ErrInvalidRefreshToken, got: %v", err)
		}
		var oauthError *OAuthError
		if !errors.As(err, &oauthError) {
			t.Errorf("Expected OAuthError, got: %v", err)
		}
		if oauthError.ErrorCode != "invalid_request" {
			t.Errorf("Expected error code 'invalid_request', got: %s", oauthError.ErrorCode)
		}
		if oauthError.ErrorDescription != "Invalid refresh token" {
			t.Errorf("Expected error description 'Invalid refresh token', got: %s", oauthError.ErrorDescription)
		}
		if refreshResult != nil {
			t.Error("Expected nil refresh result, got non-nil")
		}
	})

	t.Run("invalid access token", func(t *testing.T) {
		_, err := client.Refresh("invalid-token", WithAccessToken("invalid-token"))
		const wantErr = "invalid access token: token is malformed: token contains an invalid number of segments"
		if err.Error() != wantErr {
			t.Errorf("Expected error %q, got: %v", wantErr, err)
		}
	})

	t.Run("http error", func(t *testing.T) {
		client.issuer = server.URL + "/404"
		_, err := client.Refresh("invalid-token")
		const expected = `token refresh failed: invalid character 'N' looking for beginning of value: 404 Not Found`
		if err.Error() != expected {
			t.Fatalf("Expected error %q, got: %v", expected, err)
		}
	})
}

func TestTokenResponse(t *testing.T) {
	var tokens tokenResponse

	jsonOk := `{"access_token":"access-token","refresh_token":"refresh"}`
	if err := json.Unmarshal([]byte(jsonOk), &tokens); err != nil {
		t.Fatalf("Failed to unmarshal tokens: %v", err)
	}
	if tokens.AccessToken != "access-token" {
		t.Errorf("Expected access token 'access-token', got: %s", tokens.AccessToken)
	}
	if tokens.RefreshToken != "refresh" {
		t.Errorf("Expected refresh token 'refresh', got: %s", tokens.RefreshToken)
	}
	if tokens.OAuthError != nil {
		t.Errorf("Expected no OAuth error, got: %v", tokens.OAuthError)
	}

	jsonErr := `{"error":"invalid_request","error_description":"Invalid request"}`
	if err := json.Unmarshal([]byte(jsonErr), &tokens); err != nil {
		t.Fatalf("Failed to unmarshal OAuth error: %v", err)
	}
	if tokens.ErrorCode != "invalid_request" {
		t.Errorf("Expected error code 'invalid_request', got: %s", tokens.ErrorCode)
	}
	if tokens.ErrorDescription != "Invalid request" {
		t.Errorf("Expected error description 'Invalid request', got: %s", tokens.ErrorDescription)
	}
}
