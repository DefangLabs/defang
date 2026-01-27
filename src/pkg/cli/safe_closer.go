package cli

import (
	"io"
	"sync/atomic"
)

// SafeCloser atomically tracks a stream and closes the old one on Swap.
type SafeCloser struct {
	ptr atomic.Pointer[struct{ io.Closer }]
}

func NewSafeCloser(stream io.Closer) *SafeCloser {
	var a SafeCloser
	a.ptr.Store(&struct{ io.Closer }{stream})
	return &a
}

// Swap atomically replaces the stream and closes the old one.
func (a *SafeCloser) Swap(stream io.Closer) error {
	if old := a.ptr.Swap(&struct{ io.Closer }{stream}); old != nil {
		return old.Close()
	}
	return nil
}

// Close atomically removes and closes the current stream.
func (a *SafeCloser) Close() error {
	if stream := a.ptr.Swap(nil); stream != nil {
		return stream.Close()
	}
	return nil
}
