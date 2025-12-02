package http

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
)

var DefaultClient = newClient().StandardClient()

type Header = http.Header

// Not planning on repeating all http package constants here, but StatusOK is useful.
const StatusOK = http.StatusOK

type slogLogger struct{}

func (slogLogger) Printf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

func newClient() *retryablehttp.Client {
	c := retryablehttp.NewClient() // default client retries 4 times: 0+1+2+4+8 = 15s max
	c.Logger = slogLogger{}
	return c
}
