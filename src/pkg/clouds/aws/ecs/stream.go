package ecs

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// QueryAndTailLogGroup queries the log group from the give start time and initiates a Live Tail session.
// This function also handles the case where the log group does not exist yet.
// The caller should call `Close()` on the returned EventStream when done.
func QueryAndTailLogGroup(ctx context.Context, lgi LogGroupInput, start time.Time) (EventStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	es := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	// First call TailLogGroup once to check if the log group exists or we have another error
	eventStream, err := TailLogGroup(ctx, lgi)
	if err != nil {
		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
	}

	// Start goroutine to wait for the log group to be created and then tail it
	go func() {
		defer close(es.ch)

		if eventStream == nil {
			// If the log group does not exist yet, poll until it does
			eventStream, err = pollTailLogGroup(ctx, lgi)
			if err != nil {
				es.err = err
				return
			}
		}
		defer eventStream.Close()

		if !start.IsZero() {
			// Query the logs between the start time and now
			if err := QueryLogGroup(ctx, lgi, start, time.Now(), func(events []LogEvent) error {
				es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
					Value: types.LiveTailSessionUpdate{SessionResults: events},
				}
				return nil
			}); err != nil {
				es.err = err
				return // the caller will likely cancel the context
			}
		}

		es.pipeEvents(ctx, eventStream)
	}()

	return es, nil
}

// pollTailLogGroup polls the log group and starts the Live Tail session once it's available
func pollTailLogGroup(ctx context.Context, lgi LogGroupInput) (EventStream, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var resourceNotFound *types.ResourceNotFoundException
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			eventStream, err := TailLogGroup(ctx, lgi)
			if errors.As(err, &resourceNotFound) {
				continue // keep trying
			}
			return eventStream, err
		}
	}
}

// eventStream is an bare implementation of the EventStream interface.
type eventStream struct {
	cancel context.CancelFunc
	ch     chan types.StartLiveTailResponseStream
	err    error
}

var _ EventStream = (*eventStream)(nil)

func (es *eventStream) Close() error {
	es.cancel()
	return nil
}

func (es *eventStream) Err() error {
	return es.err
}

func (es *eventStream) Events() <-chan types.StartLiveTailResponseStream {
	return es.ch
}

// pipeEvents copies events from the given EventStream to the internal channel,
// until the context is canceled or an error occurs in the given EventStream.
func (es *eventStream) pipeEvents(ctx context.Context, eventStream EventStream) {
	for {
		// Double select to make sure context cancellation is not blocked by either the receive or send
		// See: https://stackoverflow.com/questions/60030756/what-does-it-mean-when-one-channel-uses-two-arrows-to-write-to-another-channel
		select {
		case event := <-eventStream.Events(): // blocking
			if err := eventStream.Err(); err != nil {
				es.err = err
				// Don't return, but continue to send any events to the caller so they can see the error
			}
			select {
			case es.ch <- event:
			case <-ctx.Done():
				return
			}
		case <-ctx.Done(): // blocking
			return
		}
	}
}
