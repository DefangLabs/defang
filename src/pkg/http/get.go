package http

import (
	"context"
	"net/http"
)

type Header = http.Header

const StatusOK = http.StatusOK

func GetWithContext(ctx context.Context, url string) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return DefaultClient.Do(hreq)
}

func GetWithHeader(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	hreq.Header = header
	return DefaultClient.Do(hreq)
}

func GetWithAuth(ctx context.Context, url, auth string) (*http.Response, error) {
	return GetWithHeader(ctx, url, http.Header{"Authorization": []string{auth}})
}
