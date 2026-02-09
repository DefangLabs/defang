package client

import (
	"context"
	"iter"
	"sync"
)

// ServerStreamIter adapts any ServerStream[T] (including connect-go ServerStreamForClient)
// to iter.Seq2. The stream is closed when the consumer stops iterating or the stream ends.
func ServerStreamIter[T any](stream ServerStream[T]) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		defer stream.Close()
		for stream.Receive() {
			if !yield(stream.Msg(), nil) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}

// ServerStreamIterCtx is like ServerStreamIter but also closes the stream when the
// context is canceled. This is needed for blocking streams (e.g. MockWaitStream)
// where Receive() blocks on a channel and won't return until Close() is called.
func ServerStreamIterCtx[T any](ctx context.Context, stream ServerStream[T]) iter.Seq2[*T, error] {
	return func(yield func(*T, error) bool) {
		var closeOnce sync.Once
		closeStream := func() { closeOnce.Do(func() { stream.Close() }) }

		// Close the stream when context is canceled to unblock Receive()
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				closeStream()
			case <-done:
			}
		}()
		defer close(done)
		defer closeStream()

		for stream.Receive() {
			if !yield(stream.Msg(), nil) {
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		}
	}
}
