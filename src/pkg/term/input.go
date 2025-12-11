package term

import (
	"errors"
	"io"
	"os"

	"github.com/ross96D/cancelreader"
)

var ErrClosed = errors.New("closed")

type nonBlockingStdin struct {
	cancelreader.CancelReader
}

func NewNonBlockingStdin() io.ReadCloser {
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		// Don't return os.Stdin directly as it may result in it being closed.
		// (This hack appears to work for Windows, but not on Unix.)
		fd, err := dupFd(os.Stdin.Fd())
		if err != nil {
			panic(err)
		}
		return os.NewFile(fd, "stdin-dup")
	}
	return &nonBlockingStdin{cr}
}

func (n *nonBlockingStdin) Close() error {
	if !n.CancelReader.Cancel() {
		return ErrClosed
	}
	return nil
}
