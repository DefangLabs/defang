//go:build !windows
// +build !windows

package term

func EnableANSI() func() {
	return func() {}
}
