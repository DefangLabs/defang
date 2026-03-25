package auth

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"

	defangHttp "github.com/DefangLabs/defang/src/pkg/http"
)

func TestGetAuthorizeUrl(t *testing.T) {
	orig := OpenAuthClient
	OpenAuthClient = NewClient("test-client", "https://example.com")
	t.Cleanup(func() { OpenAuthClient = orig })

	tests := []struct {
		name     string
		authType string
		parts    []string
		want     string
	}{
		{
			name:     "cli with state and challenge",
			authType: "cli",
			parts:    []string{"abc123", "xyz789"},
			want:     "https://example.com/cli/abc123/xyz789",
		},
		{
			name:     "gcp with public key",
			authType: "gcp",
			parts:    []string{"pubkeyABC="},
			want:     "https://example.com/gcp/pubkeyABC=",
		},
		{
			name:     "aws with multiple parts",
			authType: "aws",
			parts:    []string{"cross", "us-east-1", "state123", "challengeXYZ"},
			want:     "https://example.com/aws/cross/us-east-1/state123/challengeXYZ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAuthorizeUrl(tt.authType, tt.parts...)
			if got != tt.want {
				t.Errorf("GetAuthorizeUrl() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPoll(t *testing.T) {
	t.Run("success returns body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			state := r.URL.Query().Get("state")
			w.Write([]byte("code=mycode&state=" + state)) //nolint:errcheck
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		body, err := Poll(t.Context(), "mystate")
		if err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		const want = "code=mycode&state=mystate"
		if string(body) != want {
			t.Errorf("Poll() body = %q, want %q", string(body), want)
		}
	})

	t.Run("5xx retries until context cancelled", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second) // Retry client retires per second
		defer cancel()

		_, err := Poll(ctx, "state")
		if err == nil {
			t.Error("expected error after context cancellation")
		}
		if calls < 2 {
			t.Error("expected server to be called at least twice")
		}
	})

	t.Run("408 retries until context cancelled", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			w.WriteHeader(http.StatusRequestTimeout)
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := Poll(ctx, "state")
		if err == nil {
			t.Error("expected error after context cancellation")
		}
		if calls < 2 {
			t.Errorf("expected at least 2 calls for timeout retries, got %d", calls)
		}
	})

	t.Run("4xx does not retry", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		_, err := Poll(t.Context(), "state")
		if err == nil {
			t.Error("expected error for 4xx response")
		}
		if calls != 1 {
			t.Errorf("expected exactly 1 call (no retry for 4xx), got %d", calls)
		}
	})
}

func TestPollForAuthCode(t *testing.T) {
	t.Run("success extracts code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("code=myauthcode&state=mystate")) //nolint:errcheck
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		code, err := pollForAuthCode(t.Context(), "mystate")
		if err != nil {
			t.Fatalf("pollForAuthCode() error = %v", err)
		}
		if code != "myauthcode" {
			t.Errorf("pollForAuthCode() = %q, want %q", code, "myauthcode")
		}
	})

	t.Run("error field returns descriptive error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("error=access_denied&error_description=User+denied+access")) //nolint:errcheck
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		_, err := pollForAuthCode(t.Context(), "state")
		if err == nil {
			t.Fatal("expected error for auth error response")
		}
		if !strings.Contains(err.Error(), "User denied access") {
			t.Errorf("expected 'User denied access' in error, got: %v", err)
		}
	})

	t.Run("missing code field returns error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("state=somestate")) //nolint:errcheck
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		_, err := pollForAuthCode(t.Context(), "state")
		if err == nil {
			t.Fatal("expected error for missing code field")
		}
	})

	t.Run("url-encoded code value is decoded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// code value contains special chars, URL-encoded
			w.Write([]byte("code=my%2Bcode%3D&state=s")) //nolint:errcheck
		}))
		t.Cleanup(server.Close)

		orig := OpenAuthClient
		OpenAuthClient = NewClient("test", server.URL)
		t.Cleanup(func() { OpenAuthClient = orig })

		code, err := pollForAuthCode(t.Context(), "s")
		if err != nil {
			t.Fatalf("pollForAuthCode() error = %v", err)
		}
		if code != "my+code=" {
			t.Errorf("pollForAuthCode() = %q, want %q", code, "my+code=")
		}
	})
}

// fastRetryClient returns an *http.Client backed by retryablehttp with RetryMax=0
// and no wait between attempts, so "giving up after" errors are returned immediately.
func fastRetryClient() *http.Client {
	c := retryablehttp.NewClient()
	c.RetryMax = 0
	c.RetryWaitMin = 0
	c.RetryWaitMax = 0
	c.Logger = log.New(io.Discard, "", 0)
	return c.StandardClient()
}

// TestPoll_GivingUpAfterRetries_Retries verifies that when the retriablehttp transport
// returns a "giving up after X attempt(s)" error (without context cancellation), Poll
// retries the outer loop and eventually succeeds.
func TestPoll_GivingUpAfterRetries_Retries(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Close the connection without a response to produce a transport-level
			// error that retryablehttp will wrap as "giving up after 1 attempt(s)".
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Error("ResponseWriter does not support Hijack")
				http.Error(w, "hijack not supported", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack() error = %v", err)
				return
			}
			conn.Close()
			return
		}
		// Second outer-loop attempt succeeds.
		state := r.URL.Query().Get("state")
		w.Write([]byte("code=testcode&state=" + state)) //nolint:errcheck
	}))
	t.Cleanup(server.Close)

	orig := OpenAuthClient
	OpenAuthClient = NewClient("test", server.URL)
	t.Cleanup(func() { OpenAuthClient = orig })

	origClient := defangHttp.DefaultClient
	defangHttp.DefaultClient = fastRetryClient()
	t.Cleanup(func() { defangHttp.DefaultClient = origClient })

	body, err := Poll(t.Context(), "teststate")
	if err != nil {
		t.Fatalf("Poll() unexpected error = %v", err)
	}
	if !strings.Contains(string(body), "code=testcode") {
		t.Errorf("Poll() body = %q, want to contain code=testcode", string(body))
	}
	if calls < 2 {
		t.Errorf("expected at least 2 server calls (1 connection error + 1 success), got %d", calls)
	}
}

// TestPoll_GivingUpAfterRetries_ContextDone verifies that when the context is done
// (either Canceled or DeadlineExceeded), Poll does not retry even if the error
// contains "giving up after".
func TestPoll_GivingUpAfterRetries_ContextDone(t *testing.T) {
	for _, name := range []string{"context.Canceled", "context.DeadlineExceeded"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				http.Error(w, "server error", http.StatusInternalServerError)
			}))
			t.Cleanup(server.Close)

			orig := OpenAuthClient
			OpenAuthClient = NewClient("test", server.URL)
			t.Cleanup(func() { OpenAuthClient = orig })

			origClient := defangHttp.DefaultClient
			defangHttp.DefaultClient = fastRetryClient()
			t.Cleanup(func() { defangHttp.DefaultClient = origClient })

			var ctx context.Context
			if name == "context.Canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(context.Background(), time.Now())
				defer cancel()
			}

			_, err := Poll(ctx, "teststate")
			if err == nil {
				t.Fatal("expected error for done context")
			}
			// Poll must not spin retrying when the context is already done.
			if calls > 1 {
				t.Errorf("expected at most 1 server call with done context, got %d", calls)
			}
		})
	}
}
