package gcp

import (
	"context"
	"errors"
	"net"
	"net/url"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/auth/oauth2adapt"
	"github.com/DefangLabs/defang/src/pkg/term"
	"golang.org/x/oauth2"
)

const (
	tokenRefreshMaxAttempts = 4
	tokenRefreshInitialWait = 500 * time.Millisecond
)

// wrapTokenSource decorates ts with transient-error retry on refresh and
// proactive async refresh via cloud.google.com/go/auth's cached provider.
// This prevents a single slow oauth2.googleapis.com response from surfacing
// to in-flight RPCs as a fatal Unauthenticated error during long deploys.
func wrapTokenSource(ts oauth2.TokenSource) oauth2.TokenSource {
	if ts == nil {
		return nil
	}
	retrying := &retryingTokenSource{inner: ts, sleep: time.Sleep}
	cached := auth.NewCachedTokenProvider(oauth2adapt.TokenProviderFromTokenSource(retrying), nil)
	return oauth2adapt.TokenSourceFromTokenProvider(cached)
}

type retryingTokenSource struct {
	inner oauth2.TokenSource
	sleep func(time.Duration)
}

func (r *retryingTokenSource) Token() (*oauth2.Token, error) {
	wait := tokenRefreshInitialWait
	var lastErr error
	for attempt := range tokenRefreshMaxAttempts {
		if attempt > 0 {
			term.Debugf("retrying token refresh (attempt %d/%d) after transient error: %v", attempt+1, tokenRefreshMaxAttempts, lastErr)
			r.sleep(wait)
			wait *= 2
		}
		tok, err := r.inner.Token()
		if err == nil {
			return tok, nil
		}
		if !isTransientTokenError(err) {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func isTransientTokenError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeoutErr interface{ Timeout() bool }
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) {
		return true
	}
	return false
}
