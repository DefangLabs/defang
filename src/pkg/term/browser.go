package term

import (
	"bytes"
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
	return ctx, func() {
		input.Close()
		cancel()
	}
}

func OpenBrowserWithInputOnEnter(ctx context.Context, url string) (context.Context, <-chan string, func()) {
	ctx, cancel := context.WithCancel(ctx)
	input := NewNonBlockingStdin()
	inputChan := make(chan string, 1) // Buffered channel to avoid blocking goroutine

	// Handles context cancellation to ensure input is closed and goroutine exits when context is cancelled.
	// In linux Ctrl-C is handled by the signal handler, and input will not get a byte
	go func() {
		<-ctx.Done()
		input.Close()
	}()

	go func() {
		var b [1]byte
		var buf bytes.Buffer
	inputloop:
		for {
			if _, err := input.Read(b[:]); err != nil {
				break
			}
			switch b[0] {
			case 3: // Ctrl-C
				cancel()
				break inputloop
			case 10, 13: // Enter or Return
				// If the user has already entered some input, we assume browser is already opened
				// and we don't want to open the browser again.
				if buf.Len() > 0 {
					break inputloop
				}
				err := browser.OpenURL(url)
				if err != nil {
					Errorf("failed to open browser: %v", err)
				}
			default:
				buf.WriteByte(b[0]) //nolint:gosec // G602 false positive: b is [1]byte, index 0 is always valid
			}
		}
		if buf.Len() > 0 {
			inputChan <- buf.String()
		}
		close(inputChan)
	}()
	return ctx, inputChan, func() {
		input.Close()
		cancel()
	}
}
