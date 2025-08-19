package client

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
)

func PrettyError(err error) error {
	// To avoid printing the internal gRPC error code
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		term.Debug("Server error:", cerr)
		err = errors.Unwrap(cerr)
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
