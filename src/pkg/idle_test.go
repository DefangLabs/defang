package pkg

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecvWithIdleTimeout_Value(t *testing.T) {
	ch := make(chan int, 1)
	ch <- 42

	v, err := RecvWithIdleTimeout(context.Background(), ch, time.Minute)
	if err != nil {
		t.Fatalf("RecvWithIdleTimeout() error = %v, want nil", err)
	}
	if v != 42 {
		t.Fatalf("RecvWithIdleTimeout() = %d, want 42", v)
	}
}

func TestRecvWithIdleTimeout_Idle(t *testing.T) {
	ch := make(chan int) // never sent to

	_, err := RecvWithIdleTimeout(context.Background(), ch, 10*time.Millisecond)
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("RecvWithIdleTimeout() error = %v, want ErrIdleTimeout", err)
	}
}

func TestRecvWithIdleTimeout_ContextCanceled(t *testing.T) {
	ch := make(chan int)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := RecvWithIdleTimeout(ctx, ch, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RecvWithIdleTimeout() error = %v, want context.Canceled", err)
	}
}

func TestCallWithIdleTimeout_Value(t *testing.T) {
	v, err := CallWithIdleTimeout(context.Background(), time.Minute, func() (int, error) {
		return 7, nil
	})
	if err != nil {
		t.Fatalf("CallWithIdleTimeout() error = %v, want nil", err)
	}
	if v != 7 {
		t.Fatalf("CallWithIdleTimeout() = %d, want 7", v)
	}
}

func TestCallWithIdleTimeout_PropagatesError(t *testing.T) {
	wantErr := errors.New("boom")
	_, err := CallWithIdleTimeout(context.Background(), time.Minute, func() (int, error) {
		return 0, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("CallWithIdleTimeout() error = %v, want %v", err, wantErr)
	}
}

func TestCallWithIdleTimeout_Idle(t *testing.T) {
	block := make(chan struct{}) // never closed
	_, err := CallWithIdleTimeout(context.Background(), 10*time.Millisecond, func() (int, error) {
		<-block
		return 0, nil
	})
	if !errors.Is(err, ErrIdleTimeout) {
		t.Fatalf("CallWithIdleTimeout() error = %v, want ErrIdleTimeout", err)
	}
}

func TestCallWithIdleTimeout_ContextCanceled(t *testing.T) {
	block := make(chan struct{}) // never closed
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := CallWithIdleTimeout(ctx, time.Minute, func() (int, error) {
		<-block
		return 0, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CallWithIdleTimeout() error = %v, want context.Canceled", err)
	}
}
