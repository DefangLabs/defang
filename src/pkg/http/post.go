package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PostForValues issues a POST to the specified URL and returns the response body as url.Values.
func PostForValues(_url, contentType string, body io.Reader) (url.Values, error) {
	resp, err := DefaultClient.Post(_url, contentType, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	values, err := url.ParseQuery(string(bytes))
	// By default, HTTP status codes in the 2xx range are considered successful
	// and the default client will have followed any redirects.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return values, fmt.Errorf("unexpected status code: %s", resp.Status)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to parse response body: %w", err)
	}
	return values, nil
}

func PostFormWithContext(ctx context.Context, url string, data url.Values) (*http.Response, error) {
	return PostWithContext(ctx, url, "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
}

func PostWithContext(ctx context.Context, url, contentType string, body io.Reader) (*http.Response, error) {
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		hreq.Header.Set("Content-Type", contentType)
	}
	return DefaultClient.Do(hreq)
}
