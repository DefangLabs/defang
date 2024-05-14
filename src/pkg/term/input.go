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

func (n *nonBlockingStdin) Close() error {
	if !n.CancelReader.Cancel() {
		// Could not cancel; try closing the underlying handle
		if err := os.Stdin.Close(); err != nil {
			return err
		}
		return ErrClosed
	}
	return nil
}

func NewNonBlockingStdin() io.ReadCloser {
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return os.Stdin
	}
	return &nonBlockingStdin{cr}
}
