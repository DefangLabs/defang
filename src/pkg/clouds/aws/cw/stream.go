package cw

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// QueryAndTailLogGroup queries the log group from the give start time and initiates a Live Tail session.
// This function also handles the case where the log group does not exist yet.
// The caller should call `Close()` on the returned EventStream when done.
func QueryAndTailLogGroup(ctx context.Context, cw LogsClient, lgi LogGroupInput, start, end time.Time) (LiveTailStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	es := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	var tailStream LiveTailStream
	// First call TailLogGroup once to check if the log group exists or we have another error
	var err error
	tailStream, err = TailLogGroup(ctx, cw, lgi)
	if err != nil {
		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		// Doesn't exist yet, continue to poll for it
	}

	// Start goroutine to wait for the log group to be created and then tail it
	go func() {
		defer close(es.ch)

		// If the log group does not exist yet, poll until it does
		if tailStream == nil {
			var err error
			tailStream, err = pollTailLogGroup(ctx, cw, lgi)
			if err != nil {
				es.err = err
				return
			}
		}
		defer tailStream.Close()

		if !start.IsZero() {
			if end.IsZero() {
				end = time.Now()
			}
			// Query the logs between the start time and now; TODO: could use a single CloudWatch client for all queries in same region
			if err := QueryLogGroup(ctx, cw, lgi, start, end, 0, func(events []LogEvent) error {
				es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
					Value: types.LiveTailSessionUpdate{SessionResults: events},
				}
				return nil
			}); err != nil {
				es.err = err
				return // the caller will likely cancel the context
			}
		}

		// Pipe the events from the tail stream to the internal channel
		es.err = es.pipeEvents(ctx, tailStream)
	}()

	return es, nil
}

// pollTailLogGroup polls the log group and starts the Live Tail session once it's available
func pollTailLogGroup(ctx context.Context, cw StartLiveTailAPI, lgi LogGroupInput) (LiveTailStream, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var resourceNotFound *types.ResourceNotFoundException
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			eventStream, err := TailLogGroup(ctx, cw, lgi)
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

func newEventStream(cancel func()) *eventStream {
	return &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}
}

// MergeLiveTailStreams merges multiple LiveTailStreams into one.
func MergeLiveTailStreams(streams ...LiveTailStream) LiveTailStream {
	if len(streams) == 1 {
		return streams[0]
	}
	ctx, cancel := context.WithCancel(context.Background())
	es := newEventStream(func() {
		cancel()
		for _, s := range streams {
			s.Close()
		}
	})
	var wg sync.WaitGroup
	for _, s := range streams {
		wg.Add(1)
		go func() {
			defer wg.Done()
			es.err = es.pipeEvents(ctx, s)
		}()
	}
	go func() {
		wg.Wait()
		close(es.ch)
	}()
	return es
}

func NewStaticLogStream(ch <-chan LogEvent, cancel func()) EventStream[types.StartLiveTailResponseStream] {
	es := newEventStream(cancel)

	go func() {
		defer close(es.ch)
		for evt := range ch {
			es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
				Value: types.LiveTailSessionUpdate{
					SessionResults: []types.LiveTailSessionLogEvent{evt},
				},
			}
		}
	}()

	return es
}
