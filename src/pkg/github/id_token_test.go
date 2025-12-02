package github

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIdToken(t *testing.T) {
	const testToken = "test-token"

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+testToken {
			http.Error(w, "unauthorized", http.StatusForbidden)
			return
		}
		// Return the audience as the JWT
		aud := r.URL.Query().Get("audience")
		w.Write([]byte(`{"value":"` + aud + `"}`))
	}))
	t.Cleanup(s.Close)

	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", testToken)
	t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", s.URL)

	t.Run("no audience", func(t *testing.T) {
		jwt, err := GetIdToken(t.Context(), "")
		if err != nil {
			t.Fatal(err)
		}
		if jwt != "" {
			t.Error("wrong JWT/audience")
		}
	})

	t.Run("with audience", func(t *testing.T) {
		jwt, err := GetIdToken(t.Context(), "https://github.com/DefangLabs")
		if err != nil {
			t.Fatal(err)
		}
		if jwt != "https://github.com/DefangLabs" {
			t.Error("wrong JWT/audience")
		}
	})

	t.Run("with other query params", func(t *testing.T) {
		t.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", s.URL+"?foo=bar")
		jwt, err := GetIdToken(t.Context(), "sts.amazonaws.com")
		if err != nil {
			t.Fatal(err)
		}
		if jwt != "sts.amazonaws.com" {
			t.Error("wrong JWT/audience")
		}
	})
}
