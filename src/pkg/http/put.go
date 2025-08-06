package http

import (
	"context"
	"io"
	"net/http"
)

// Put issues a PUT to the specified URL.
//
// Caller should close resp.Body when done reading from it.
//
// If the provided body is an io.Closer, it is closed after the
// request.
//
// To set custom headers, use NewRequest and DefaultClient.Do.
//
// See the Client.Do method documentation for details on how redirects
// are handled.
func Put(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	return PutWithHeader(ctx, url, http.Header{"Content-Type": []string{contentType}}, body)
}

func PutWithHeader(ctx context.Context, url string, header http.Header, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header = header
	return DefaultClient.Do(req)
}
