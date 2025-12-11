//go:build windows
// +build windows

package term

import (
	"golang.org/x/sys/windows"
)

func dupFd(fd uintptr) (uintptr, error) {
	var newH windows.Handle
	err := windows.DuplicateHandle(
		windows.CurrentProcess(), // source process
		windows.Handle(fd),       // source handle
		windows.CurrentProcess(), // target process
		&newH,                    // duplicated handle
		0,                        // access (0 = same)
		false,                    // inheritable?
		windows.DUPLICATE_SAME_ACCESS,
	)
	return uintptr(newH), err
}
