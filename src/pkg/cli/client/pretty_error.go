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
		// the call was for) that wraps it. Unwrap still reaches whatever error
		// the connect.Error itself wrapped, so errors.Is/As keep working.
		err = &prettyError{
			msg: strings.Replace(err.Error(), cerr.Error(), cerr.Message(), 1),
			err: errors.Unwrap(cerr),
		}
	}
	if IsNetworkError(err) {
		return fmt.Errorf("%w; please check network settings and try again", err)
	}
	return err
}

// prettyError carries a display message distinct from its wrapped error's own
// Error() text, while still exposing that error through Unwrap.
type prettyError struct {
	msg string
	err error
}

func (e *prettyError) Error() string { return e.msg }
func (e *prettyError) Unwrap() error { return e.err }

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
