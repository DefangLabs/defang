package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostForValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an error to test the error handling on /error
		if r.URL.Path == "/error" {
			http.Error(w, "error", http.StatusNotFound) // not retried
			return
		}
		// Return a redirect to test the error handling on unexpected status codes
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Write([]byte("foo=bar&baz=qux"))
	}))
	t.Cleanup(server.Close)

	t.Run("success", func(t *testing.T) {
		values, err := PostForValues(server.URL, "application/text", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := values.Get("foo"), "bar"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("redirect", func(t *testing.T) {
		values, err := PostForValues(server.URL+"/redirect", "application/text", nil)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := values.Get("foo"), "bar"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("error", func(t *testing.T) {
		_, err := PostForValues(server.URL+"/error", "application/text", nil)
		if err == nil {
			t.Fatal("expected an error")
		}
		if got, want := err.Error(), "unexpected status code: 404 Not Found"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
