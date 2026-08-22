package pkg

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrIdleTimeout is returned when a stream read has not produced any data (and no error)
// within the configured idle window. Long-lived server streams (e.g. CloudWatch Live Tail,
// GCP TailLogEntries) can stall on a half-dead connection without ever closing or erroring,
// so callers need an idle deadline distinct from the overall context deadline to detect and
// recover from that instead of blocking indefinitely.
var ErrIdleTimeout = errors.New("idle timeout: no data received")

// RecvWithIdleTimeout receives a single value from ch, returning ErrIdleTimeout if nothing
// arrives within d, or ctx.Err() if ctx is done first. If ch is closed (and drained), it
// returns io.EOF, matching the two-value receive form's "ok" signal rather than silently
// handing back T's zero value forever. Use this for channel-based reads (e.g. an AWS SDK
// EventStream's Events() channel) where wrapping the read in a goroutine would be wasteful.
func RecvWithIdleTimeout[T any](ctx context.Context, ch <-chan T, d time.Duration) (T, error) {
	var zero T
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case v, ok := <-ch:
		if !ok {
			return zero, io.EOF
		}
		return v, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, ErrIdleTimeout
	}
}

type idleResult[T any] struct {
	val T
	err error
}

// CallWithIdleTimeout runs fn and returns its result, or ErrIdleTimeout if fn does not
// return within d, or ctx.Err() if ctx is done first. Use this for synchronous, blocking
// calls (e.g. a gRPC stream's Recv()) that don't expose a channel to select on.
//
// If the timeout (or ctx) fires first, fn keeps running in the background and its result is
// discarded; the caller is expected to close/cancel whatever fn is blocked on so that
// goroutine doesn't run forever.
func CallWithIdleTimeout[T any](ctx context.Context, d time.Duration, fn func() (T, error)) (T, error) {
	ch := make(chan idleResult[T], 1)
	go func() {
		v, err := fn()
		ch <- idleResult[T]{v, err}
	}()

	var zero T
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.val, r.err
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, ErrIdleTimeout
	}
}
