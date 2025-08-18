package http

import (
	"fmt"
	"log/slog"

	"github.com/hashicorp/go-retryablehttp"
)

var DefaultClient = newClient().StandardClient()

type slogLogger struct{}

func (slogLogger) Printf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

func newClient() *retryablehttp.Client {
	c := retryablehttp.NewClient() // default client retries 4 times: 0+1+2+4+8 = 15s max
	c.Logger = slogLogger{}
	return c
}
