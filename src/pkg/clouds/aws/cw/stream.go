package cw

import (
	"context"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// QueryAndTailLogGroup queries the log group from the given start time and initiates a Live Tail session.
// This function also handles the case where the log group does not exist yet.
func QueryAndTailLogGroup(ctx context.Context, cwClient LogsClient, lgi LogGroupInput, start, end time.Time) (iter.Seq2[LogEvent, error], error) {
	tailIter, err := TailLogGroup(ctx, cwClient, lgi)
	if err != nil {
		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		// Doesn't exist yet, continue to poll for it
	}

	return func(yield func(LogEvent, error) bool) {
		// If the log group does not exist yet, poll until it does
		if tailIter == nil {
			var err error
			tailIter, err = pollTailLogGroup(ctx, cwClient, lgi)
			if err != nil {
				yield(LogEvent{}, err)
				return
			}
		}

		// Query historical logs
		if !start.IsZero() {
			if end.IsZero() {
				end = time.Now()
			}
			queryIter, err := QueryLogGroup(ctx, cwClient, lgi, start, end, 0)
			if err == nil {
				for events, err := range queryIter {
					if err != nil {
						yield(LogEvent{}, err)
						return
					}
					for _, evt := range events {
						if !yield(evt, nil) {
							return
						}
					}
				}
			}
		}

		// Tail live logs
		for evt, err := range tailIter {
			if !yield(evt, err) {
				return
			}
			if err != nil {
				return
			}
		}
	}, nil
}

// pollTailLogGroup polls the log group and starts the Live Tail session once it's available
func pollTailLogGroup(ctx context.Context, cw StartLiveTailAPI, lgi LogGroupInput) (iter.Seq2[LogEvent, error], error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	var resourceNotFound *types.ResourceNotFoundException
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			logIter, err := TailLogGroup(ctx, cw, lgi)
			if errors.As(err, &resourceNotFound) {
				continue // keep trying
			}
			return logIter, err
		}
	}
}

// QueryAndTailLogGroups queries and tails multiple log groups concurrently.
// Events from different groups are interleaved (not merge-sorted).
func QueryAndTailLogGroups(ctx context.Context, cwClient LogsClient, start, end time.Time, lgis ...LogGroupInput) (iter.Seq2[LogEvent, error], error) {
	ctx, cancel := context.WithCancel(ctx)

	type result struct {
		evt LogEvent
		err error
	}
	ch := make(chan result)

	var wg sync.WaitGroup
	var lastErr error
	for _, lgi := range lgis {
		logIter, err := QueryAndTailLogGroup(ctx, cwClient, lgi, start, end)
		if err != nil {
			lastErr = err
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for evt, err := range logIter {
				select {
				case ch <- result{evt, err}:
				case <-ctx.Done():
					return
				}
				if err != nil {
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	if lastErr != nil {
		cancel()
		return nil, lastErr
	}

	return func(yield func(LogEvent, error) bool) {
		defer cancel()
		for r := range ch {
			if !yield(r.evt, r.err) {
				return
			}
			if r.err != nil {
				return
			}
		}
	}, nil
}
