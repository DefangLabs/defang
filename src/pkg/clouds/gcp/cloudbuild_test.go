package gcp

import (
	"context"
	"errors"
	"testing"
	"time"

	cloudbuildpb "cloud.google.com/go/cloudbuild/apiv1/v2/cloudbuildpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestPollBuildWithRetry_SuccessNoRetry(t *testing.T) {
	want := &cloudbuildpb.Build{Id: "ok"}
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		return want, nil
	}
	got, err := pollBuildWithRetry(t.Context(), poll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if calls != 1 {
		t.Errorf("poll called %d times, want 1", calls)
	}
}

func TestPollBuildWithRetry_RetriesOnTransient(t *testing.T) {
	prev := pollBuildInitialWaitForTest
	pollBuildInitialWaitForTest = time.Microsecond
	t.Cleanup(func() { pollBuildInitialWaitForTest = prev })

	want := &cloudbuildpb.Build{Id: "ok"}
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		if calls < 3 {
			return nil, grpcstatus.Error(codes.DeadlineExceeded, "deadline")
		}
		return want, nil
	}
	got, err := pollBuildWithRetry(t.Context(), poll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if calls != 3 {
		t.Errorf("poll called %d times, want 3", calls)
	}
}

func TestPollBuildWithRetry_GivesUpAfterMax(t *testing.T) {
	prev := pollBuildInitialWaitForTest
	pollBuildInitialWaitForTest = time.Microsecond
	t.Cleanup(func() { pollBuildInitialWaitForTest = prev })

	transient := grpcstatus.Error(codes.Unavailable, "unavailable")
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		return nil, transient
	}
	_, err := pollBuildWithRetry(t.Context(), poll)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != pollBuildMaxAttempts {
		t.Errorf("poll called %d times, want %d", calls, pollBuildMaxAttempts)
	}
}

func TestPollBuildWithRetry_RetriesOnTransientAuth(t *testing.T) {
	prev := pollBuildInitialWaitForTest
	pollBuildInitialWaitForTest = time.Microsecond
	t.Cleanup(func() { pollBuildInitialWaitForTest = prev })

	// A 503 from fabric while refreshing per-RPC creds surfaces as Unauthenticated.
	authFlake := grpcstatus.Error(codes.Unauthenticated, "per-RPC creds failed due to error: credentials: status code 503: upstream connect error")
	want := &cloudbuildpb.Build{Id: "ok"}
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		if calls < 3 {
			return nil, authFlake
		}
		return want, nil
	}
	got, err := pollBuildWithRetry(t.Context(), poll)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if calls != 3 {
		t.Errorf("poll called %d times, want 3", calls)
	}
}

func TestPollBuildWithRetry_GivesUpAfterMaxOnTransientAuth(t *testing.T) {
	prev := pollBuildInitialWaitForTest
	pollBuildInitialWaitForTest = time.Microsecond
	t.Cleanup(func() { pollBuildInitialWaitForTest = prev })

	authFlake := grpcstatus.Error(codes.Unauthenticated, "upstream connect error or disconnect/reset before headers")
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		return nil, authFlake
	}
	_, err := pollBuildWithRetry(t.Context(), poll)
	if !errors.Is(err, authFlake) {
		t.Errorf("got error %v, want %v", err, authFlake)
	}
	if calls != pollBuildMaxAttempts {
		t.Errorf("poll called %d times, want %d", calls, pollBuildMaxAttempts)
	}
}

func TestPollBuildWithRetry_DoesNotRetryPermanent(t *testing.T) {
	perm := grpcstatus.Error(codes.PermissionDenied, "denied")
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		return nil, perm
	}
	_, err := pollBuildWithRetry(t.Context(), poll)
	if !errors.Is(err, perm) {
		t.Errorf("got error %v, want %v", err, perm)
	}
	if calls != 1 {
		t.Errorf("poll called %d times, want 1 (no retry)", calls)
	}
}

func TestPollBuildWithRetry_ParentCtxCanceledMidRetry(t *testing.T) {
	prev := pollBuildInitialWaitForTest
	pollBuildInitialWaitForTest = 50 * time.Millisecond
	t.Cleanup(func() { pollBuildInitialWaitForTest = prev })

	ctx, cancel := context.WithCancel(t.Context())
	transient := grpcstatus.Error(codes.DeadlineExceeded, "deadline")
	calls := 0
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		calls++
		if calls == 1 {
			// After the first transient error, cancel the parent ctx so the
			// retry-wait select picks ctx.Done().
			go cancel()
		}
		return nil, transient
	}
	_, err := pollBuildWithRetry(ctx, poll)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestPollBuildWithRetry_ParentCtxDoneSurfacesAsCtxErr(t *testing.T) {
	// When poll returns a transient error but the parent ctx is already done,
	// surface the parent's ctx.Err rather than the downstream error — that's
	// how we distinguish "user/caller timed out" from "downstream gax 10s fired."
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel
	transient := grpcstatus.Error(codes.DeadlineExceeded, "deadline")
	poll := func(ctx context.Context, _ ...gax.CallOption) (*cloudbuildpb.Build, error) {
		return nil, transient
	}
	_, err := pollBuildWithRetry(ctx, poll)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestIsTransientPollError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("oops"), false},
		{"deadline", grpcstatus.Error(codes.DeadlineExceeded, ""), true},
		{"unavailable", grpcstatus.Error(codes.Unavailable, ""), true},
		{"internal", grpcstatus.Error(codes.Internal, ""), true},
		{"resource exhausted", grpcstatus.Error(codes.ResourceExhausted, ""), true},
		{"permission denied", grpcstatus.Error(codes.PermissionDenied, ""), false},
		{"unauthenticated", grpcstatus.Error(codes.Unauthenticated, ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientPollError(tt.err); got != tt.want {
				t.Errorf("isTransientPollError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsTransientAuthError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain", errors.New("oops"), false},
		{"bare unauthenticated", grpcstatus.Error(codes.Unauthenticated, "invalid token"), false},
		{"per-RPC creds", grpcstatus.Error(codes.Unauthenticated, "per-RPC creds failed due to error"), true},
		{"upstream connect", grpcstatus.Error(codes.Unauthenticated, "credentials: status code 503: upstream connect error"), true},
		{"real fabric 503", grpcstatus.Error(codes.Unauthenticated, "transport: per-RPC creds failed due to error: credentials: status code 503: upstream connect error or disconnect/reset before headers. retried and the latest reset reason: remote connection failure, transport failure reason: delayed connect error: Connection refused"), true},
		{"other code with marker", grpcstatus.Error(codes.Unavailable, "upstream connect error"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientAuthError(tt.err); got != tt.want {
				t.Errorf("isTransientAuthError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
