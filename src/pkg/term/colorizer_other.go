//go:build !windows
// +build !windows

package term

import "os"

func EnableANSI() func() {
	return func() {}
}

func isTerminal() bool {
	return os.Getenv("TERM") != ""
}
