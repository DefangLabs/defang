package ecs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

type collectionStream struct {
	cancel   context.CancelFunc
	ch       chan types.StartLiveTailResponseStream
	outputCh chan types.StartLiveTailResponseStream
	ctx      context.Context // derived from the context passed to TailLogGroups
	errCh    chan error
	streams  []EventStream

	err error

	lock sync.Mutex
	wg   sync.WaitGroup
}

func newCollectionStream(ctx context.Context) *collectionStream {
	child, cancel := context.WithCancel(ctx)
	cs := &collectionStream{
		cancel:   cancel,
		ch:       make(chan types.StartLiveTailResponseStream, 10), // max number of loggroups to query
		outputCh: make(chan types.StartLiveTailResponseStream),
		ctx:      child,
		errCh:    make(chan error, 1),
	}

	go func() {
		defer close(cs.outputCh)
		for {
			select {
			case e, ok := <-cs.ch:
				// This would make sure the goroutine exits after close is called
				if !ok {
					return
				}
				cs.outputCh <- e
			case err := <-cs.errCh:
				cs.err = err
				cs.outputCh <- nil
			case <-cs.ctx.Done():
				return
			}
		}
	}()

	return cs
}

func (c *collectionStream) addAndStart(s EventStream, since time.Time, lgi LogGroupInput) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.streams = append(c.streams, s)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if !since.IsZero() {
			// Query the logs between the start time and now
			query := &CWLogGroupQuery{
				LogGroupInput: lgi,
				Start:         since,
				End:           time.Now(),
			}
			for query.HasNext() {
				if events, err := query.Next(c.ctx); err != nil {
					c.errCh <- err // the caller will likely cancel the context
				} else {
					c.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
						Value: types.LiveTailSessionUpdate{SessionResults: events},
					}
				}
			}
		}
		for {
			// Double select to make sure context cancellation is not blocked by either the receive or send
			// See: https://stackoverflow.com/questions/60030756/what-does-it-mean-when-one-channel-uses-two-arrows-to-write-to-another-channel
			select {
			case e := <-s.Events(): // blocking
				if err := s.Err(); err != nil {
					select {
					case c.errCh <- err:
					case <-c.ctx.Done():
					}
					return
				}
				select {
				case c.ch <- e:
				case <-c.ctx.Done():
					return
				}
			case <-c.ctx.Done(): // blocking
				return
			}
		}
	}()
}

func (c *collectionStream) Close() error {
	c.cancel()
	c.wg.Wait() // Only close the channels after all goroutines have exited
	close(c.ch)
	close(c.errCh)

	var errs []error
	for _, s := range c.streams {
		err := s.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...) // nil if no errors
}

func (c *collectionStream) Events() <-chan types.StartLiveTailResponseStream {
	return c.outputCh
}

func (c *collectionStream) Err() error {
	return c.err
}
