//go:build windows
// +build windows

package term

import (
	"github.com/muesli/termenv"
)

func EnableANSI() func() {
	mode, err := termenv.EnableWindowsANSIConsole()
	if err != nil {
		return func() {}
	}
	return func() {
		termenv.RestoreWindowsConsole(mode)
	}
}
