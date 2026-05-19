package gcp

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type stubTokenSource struct {
	calls int
	errs  []error
	tok   *oauth2.Token
}

func (s *stubTokenSource) Token() (*oauth2.Token, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		return nil, s.errs[i]
	}
	return s.tok, nil
}

func newStub(tok *oauth2.Token, errs ...error) *stubTokenSource {
	return &stubTokenSource{errs: errs, tok: tok}
}

func TestRetryingTokenSource_SuccessNoRetry(t *testing.T) {
	want := &oauth2.Token{AccessToken: "ok"}
	r := &retryingTokenSource{inner: newStub(want), sleep: func(time.Duration) {}}
	got, err := r.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRetryingTokenSource_RetriesOnTransient(t *testing.T) {
	want := &oauth2.Token{AccessToken: "ok"}
	transient := &url.Error{Op: "Post", URL: "https://oauth2.googleapis.com/token", Err: context.DeadlineExceeded}
	stub := newStub(want, transient, transient)

	slept := 0
	r := &retryingTokenSource{inner: stub, sleep: func(time.Duration) { slept++ }}
	got, err := r.Token()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if stub.calls != 3 {
		t.Errorf("inner.Token called %d times, want 3", stub.calls)
	}
	if slept != 2 {
		t.Errorf("sleep called %d times, want 2", slept)
	}
}

func TestRetryingTokenSource_GivesUpAfterMaxAttempts(t *testing.T) {
	transient := &url.Error{Op: "Post", URL: "https://oauth2.googleapis.com/token", Err: context.DeadlineExceeded}
	stub := newStub(nil, transient, transient, transient, transient, transient)

	r := &retryingTokenSource{inner: stub, sleep: func(time.Duration) {}}
	_, err := r.Token()
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if stub.calls != tokenRefreshMaxAttempts {
		t.Errorf("inner.Token called %d times, want %d", stub.calls, tokenRefreshMaxAttempts)
	}
}

func TestRetryingTokenSource_DoesNotRetryPermanent(t *testing.T) {
	perm := errors.New("invalid_grant: refresh token expired")
	stub := newStub(nil, perm)

	r := &retryingTokenSource{inner: stub, sleep: func(time.Duration) {}}
	_, err := r.Token()
	if !errors.Is(err, perm) {
		t.Errorf("got error %v, want %v", err, perm)
	}
	if stub.calls != 1 {
		t.Errorf("inner.Token called %d times, want 1 (no retry)", stub.calls)
	}
}

func TestIsTransientTokenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context deadline", context.DeadlineExceeded, true},
		{"wrapped context deadline", &url.Error{Op: "Post", URL: "https://x", Err: context.DeadlineExceeded}, true},
		{"net op error", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		{"url error wrapping plain", &url.Error{Op: "Post", URL: "https://x", Err: errors.New("EOF")}, true},
		{"permanent oauth", errors.New("invalid_grant"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientTokenError(tt.err); got != tt.want {
				t.Errorf("isTransientTokenError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestWrapTokenSource_NilPassesThrough(t *testing.T) {
	if got := wrapTokenSource(nil); got != nil {
		t.Errorf("wrapTokenSource(nil) = %v, want nil", got)
	}
}

func TestWrapTokenSource_CachesAcrossCalls(t *testing.T) {
	want := &oauth2.Token{AccessToken: "ok", Expiry: time.Now().Add(time.Hour)}
	stub := newStub(want)

	ts := wrapTokenSource(stub)
	for range 3 {
		got, err := ts.Token()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.AccessToken != want.AccessToken {
			t.Errorf("got %q, want %q", got.AccessToken, want.AccessToken)
		}
	}
	if stub.calls != 1 {
		t.Errorf("inner.Token called %d times, want 1 (cached)", stub.calls)
	}
}
