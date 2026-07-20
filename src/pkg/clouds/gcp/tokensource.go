package gcp

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
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
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeoutErr interface{ Timeout() bool }
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return true
	}
	// oauth2/google's AuthenticationError (and net errors) expose Temporary()
	// for the retryable server-side statuses 500, 503, 408, 429.
	var temporaryErr interface{ Temporary() bool }
	if errors.As(err, &temporaryErr) && temporaryErr.Temporary() {
		return true
	}
	// A standard OAuth2 token refresh (stored-credential path) surfaces a
	// non-2xx from the token endpoint as a typed RetrieveError; retry the same
	// server-side statuses.
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) && retrieveErr.Response != nil && isRetryableStatus(retrieveErr.Response.StatusCode) {
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
	// STS / external-account token exchanges (Workload Identity Federation)
	// report a non-2xx as a plain error ("...: status code 503: <body>") with no
	// typed status, so match the retryable server-side codes on the message.
	// This is the 503-from-fabric case that otherwise surfaces to in-flight RPCs
	// as a fatal Unauthenticated during a deploy.
	msg := err.Error()
	for _, code := range retryableStatusMarkers {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

// retryableStatusMarkers are the message fragments the oauth2/auth libraries
// emit for server-side statuses worth retrying (500, 503, 408, 429).
var retryableStatusMarkers = []string{
	"status code 500",
	"status code 503",
	"status code 408",
	"status code 429",
}

func isRetryableStatus(code int) bool {
	switch code {
	case 500, 503, 408, 429:
		return true
	}
	return false
}
