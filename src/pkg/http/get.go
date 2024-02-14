package http

import (
	"context"
	"net/http"
)

func GetWithAuth(ctx context.Context, url, auth string) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Authorization", auth)
	return http.DefaultClient.Do(hreq)
}
