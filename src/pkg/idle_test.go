package pkg

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestRecvWithIdleTimeout(t *testing.T) {
	tests := []struct {
		name    string
		makeCh  func() <-chan int
		ctx     func() context.Context
		timeout time.Duration
		want    int
		wantErr error
	}{
		{
			name: "value",
			makeCh: func() <-chan int {
				ch := make(chan int, 1)
				ch <- 42
				return ch
			},
			ctx:     context.Background,
			timeout: time.Minute,
			want:    42,
		},
		{
			name: "closed channel",
			makeCh: func() <-chan int {
				ch := make(chan int)
				close(ch)
				return ch
			},
			ctx:     context.Background,
			timeout: time.Minute,
			wantErr: io.EOF,
		},
		{
			name:    "idle timeout",
			makeCh:  func() <-chan int { return make(chan int) }, // never sent to
			ctx:     context.Background,
			timeout: 10 * time.Millisecond,
			wantErr: ErrIdleTimeout,
		},
		{
			name:   "context canceled",
			makeCh: func() <-chan int { return make(chan int) },
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			timeout: time.Minute,
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RecvWithIdleTimeout(tt.ctx(), tt.makeCh(), tt.timeout)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("RecvWithIdleTimeout() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("RecvWithIdleTimeout() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("RecvWithIdleTimeout() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCallWithIdleTimeout(t *testing.T) {
	errBoom := errors.New("boom")
	block := make(chan struct{}) // never closed

	tests := []struct {
		name    string
		fn      func() (int, error)
		ctx     func() context.Context
		timeout time.Duration
		want    int
		wantErr error
	}{
		{
			name:    "value",
			fn:      func() (int, error) { return 7, nil },
			ctx:     context.Background,
			timeout: time.Minute,
			want:    7,
		},
		{
			name:    "propagated error",
			fn:      func() (int, error) { return 0, errBoom },
			ctx:     context.Background,
			timeout: time.Minute,
			wantErr: errBoom,
		},
		{
			name:    "idle timeout",
			fn:      func() (int, error) { <-block; return 0, nil },
			ctx:     context.Background,
			timeout: 10 * time.Millisecond,
			wantErr: ErrIdleTimeout,
		},
		{
			name: "context canceled",
			fn:   func() (int, error) { <-block; return 0, nil },
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			timeout: time.Minute,
			wantErr: context.Canceled,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CallWithIdleTimeout(tt.ctx(), tt.timeout, tt.fn)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CallWithIdleTimeout() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CallWithIdleTimeout() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("CallWithIdleTimeout() = %d, want %d", got, tt.want)
			}
		})
	}
}
