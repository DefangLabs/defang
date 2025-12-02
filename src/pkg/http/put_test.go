package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPutRetries(t *testing.T) {
	const body = "test"
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		if b, err := io.ReadAll(r.Body); err != nil || string(b) != body {
			t.Error("expected body to be read")
		}
	}))
	t.Cleanup(server.Close)

	resp, err := Put(t.Context(), server.URL, "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}
