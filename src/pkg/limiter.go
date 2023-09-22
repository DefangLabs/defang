package pkg

import (
	"net/http"

	"golang.org/x/time/rate"
)

const (
	BurstLimit = 50 // TODO: should depend on the actual chosen limit?
)

func NewLimiterMiddleware(limit rate.Limit, next http.Handler) http.Handler {
	limiter := rate.NewLimiter(limit, BurstLimit) // TODO: per peer rate limiting
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
