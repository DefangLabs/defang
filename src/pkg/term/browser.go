package term

import (
	"context"

	"github.com/pkg/browser"
)

func OpenBrowserOnEnter(ctx context.Context, url string) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	input := NewNonBlockingStdin()
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
