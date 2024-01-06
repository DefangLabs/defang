package http

import (
	"context"
	"io"
	"net/http"
	"net/url"
)

func Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "PUT", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return http.DefaultClient.Do(req)
}

func RemoveQueryParam(qurl string) string {
	u, err := url.Parse(qurl)
	if err != nil {
		return qurl
	}
	u.RawQuery = ""
	return u.String()
}
