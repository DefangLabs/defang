package term

import (
	"context"
	"io"

	"github.com/pkg/browser"
)

func init() {
	// Suppress browser package output
	browser.Stdout = io.Discard
	browser.Stderr = io.Discard
}

func OpenBrowserOnEnter(ctx context.Context, url string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	input := NewNonBlockingStdin()

	// Handles context cancellation to ensure input is closed and goroutine exits when context is cancelled.
	// In linux Ctrl-C is handled by the signal handler, and input will not get a byte
	go func() {
		<-ctx.Done()
		input.Close()
	}()

	go func() {
		var b [1]byte
		for {
			if _, err := input.Read(b[:]); err != nil {
				return // exit goroutine
			}
			switch b[0] {
			case 3: // Ctrl-C
				cancel()
				return
			case 10, 13: // Enter or Return
				err := browser.OpenURL(url)
				if err != nil {
					Errorf("failed to open browser: %v", err)
				}
			default:
			}
		}
	}()
	return ctx, cancel
}
