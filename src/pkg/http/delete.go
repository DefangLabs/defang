package http

import (
	"context"
	"net/http"
)

func DeleteWithHeader(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	hreq.Header = header
	return DefaultClient.Do(hreq)
}

func DeleteWithAuth(ctx context.Context, url, auth string) (*http.Response, error) {
	return DeleteWithHeader(ctx, url, http.Header{"Authorization": []string{auth}})
}
