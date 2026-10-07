package client

import (
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func PrettyError(err error) error {
	// To avoid printing the internal gRPC error code
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		term.Debug("Server error:", cerr)
		// Replace just the connect error's own "<code>: <message>" text with its
		// bare message, keeping any outer context (e.g. which recipe or project
		// the call was for) that wraps it.
		err = errors.New(strings.Replace(err.Error(), cerr.Error(), cerr.Message(), 1))
	}
	if IsNetworkError(err) {
		return fmt.Errorf("%w; please check network settings and try again", err)
	}
	return err
}

func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	lastColon := strings.LastIndexByte(errStr, ':')
	switch errStr[lastColon+1:] { // +1 to skip the colon and handle the case where there is no colon
	case " connection refused",
		" i/o timeout",
		" network is unreachable",
		" no such host",
		" unexpected EOF",
		" device or resource busy":
		return true
	}
	return false
}
