package spinner

import (
	"context"
	"fmt"
	"time"
)

var SpinnerChars = `-\|/`

type Spinner struct {
	cnt int
}

func New() *Spinner {
	return &Spinner{}
}

func (s *Spinner) Next() string {
	s.cnt++
	runes := []rune(SpinnerChars)
	return string([]rune{runes[s.cnt%len(runes)], '\b'})
}

func (s *Spinner) Start(ctx context.Context) (context.Context, context.CancelFunc) {
	cancelCtx, cancel := context.WithCancel(ctx)
	go func(spinnerCtx context.Context) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-spinnerCtx.Done():
				return
			case <-ticker.C:
				fmt.Print(s.Next())
			}
		}
	}(cancelCtx)

	return cancelCtx, cancel
}
