package http

import (
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/hashicorp/go-retryablehttp"
)

var DefaultClient = newClient().StandardClient()

type termLogger struct{}

func (termLogger) Printf(format string, args ...interface{}) {
	term.Debugf(format, args...)
}

func newClient() *retryablehttp.Client {
	c := retryablehttp.NewClient() // default client retries 4 times: 1+2+4+8 = 15s max
	c.Logger = termLogger{}
	return c
}
