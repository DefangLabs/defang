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
func QueryAndTailLogGroup(ctx context.Context, lgi LogGroupInput, start, end time.Time, doTail bool) (LiveTailStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	es := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	var tailStream LiveTailStream
	if doTail {
		// First call TailLogGroup once to check if the log group exists or we have another error
		var err error
		tailStream, err = TailLogGroup(ctx, lgi)
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) {
				return nil, err
			}
			// Doesn't exist yet, continue to poll for it
		}
	}

	// Start goroutine to wait for the log group to be created and then tail it
	go func() {
		defer close(es.ch)

		if doTail {
			// If the log group does not exist yet, poll until it does
			if tailStream == nil {
				var err error
				tailStream, err = pollTailLogGroup(ctx, lgi)
				if err != nil {
					es.err = err
					return
				}
			}
			defer tailStream.Close()
		}

		if !start.IsZero() {
			if end.IsZero() {
				end = time.Now()
			}
			// Query the logs between the start time and now; TODO: could use a single CloudWatch client for all queries in same region
			if err := QueryLogGroup(ctx, lgi, start, end, func(events []LogEvent) error {
				es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
					Value: types.LiveTailSessionUpdate{SessionResults: events},
				}
				return nil
			}); err != nil {
				es.err = err
				return // the caller will likely cancel the context
			}
		}

		if doTail {
			// Pipe the events from the tail stream to the internal channel
			es.err = es.pipeEvents(ctx, tailStream)
		}
	}()

	return es, nil
}

// pollTailLogGroup polls the log group and starts the Live Tail session once it's available
func pollTailLogGroup(ctx context.Context, lgi LogGroupInput) (LiveTailStream, error) {
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

var _ LiveTailStream = (*eventStream)(nil)

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
func (es *eventStream) pipeEvents(ctx context.Context, tailStream LiveTailStream) error {
	for {
		// Double select to make sure context cancellation is not blocked by either the receive or send
		// See: https://stackoverflow.com/questions/60030756/what-does-it-mean-when-one-channel-uses-two-arrows-to-write-to-another-channel
		select {
		case event := <-tailStream.Events(): // blocking
			if err := tailStream.Err(); err != nil {
				return err
			}
			if event == nil {
				return nil
			}
			select {
			case es.ch <- event:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done(): // blocking
			return ctx.Err()
		}
	}
}
