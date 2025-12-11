//go:build !windows
// +build !windows

package term

import (
	"golang.org/x/sys/unix"
)

func dupFd(fd uintptr) (uintptr, error) {
	nfd, err := unix.Dup(int(fd))
	return uintptr(nfd), err
}
