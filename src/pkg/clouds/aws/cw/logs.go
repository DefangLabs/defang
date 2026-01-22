package cw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/smithy-go/ptr"
)

// Task ARN						arn:aws:ecs:us-west-2:123456789012:task/CLUSTER_NAME/2cba912d5eb14ffd926f6992b054f3bf
// Cluster ARN					arn:aws:ecs:us-west-2:123456789012:cluster/CLUSTER_NAME
// LogGroup ARN					arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME:*
// LogGroup ID					arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME
// LogStream ("awslogs")		PREFIX/CONTAINER/2cba912d5eb14ffd926f6992b054f3bf
// LogStream ("awsfirelens")	PREFIX/CONTAINER-firelens-2cba912d5eb14ffd926f6992b054f3bf

func getLogGroupIdentifier(arnOrId string) string {
	return strings.TrimSuffix(arnOrId, ":*")
}

type LogsClient interface {
	FilterLogEventsAPI
	StartLiveTailAPI
}

func QueryAndTailLogGroups(ctx context.Context, cwClient LogsClient, start, end time.Time, logGroups ...LogGroupInput) (LiveTailStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	e := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	// We must close the channel when all log groups are done
	var wg sync.WaitGroup
	var err error
	for _, lgi := range logGroups {
		var es LiveTailStream
		es, err = QueryAndTailLogGroup(ctx, cwClient, lgi, start, end)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer es.Close()
			defer wg.Done()
			// FIXME: this should *merge* the events from all log groups
			e.err = e.pipeEvents(ctx, es)
		}()
	}

	go func() {
		wg.Wait()
		close(e.ch)
	}()

	if err != nil {
		cancel() // abort any goroutines (caller won't call Close)
		return nil, err
	}

	return e, nil
}

// LogGroupInput is like cloudwatchlogs.StartLiveTailInput but with only one LogGroup and one LogStream prefix.
type LogGroupInput struct {
	LogGroupARN           string
	LogStreamNames        []string
	LogStreamNamePrefix   string
	LogEventFilterPattern string
}

type StartLiveTailAPI interface {
	StartLiveTail(ctx context.Context, params *cloudwatchlogs.StartLiveTailInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.StartLiveTailOutput, error)
}

func TailLogGroup(ctx context.Context, cwClient StartLiveTailAPI, input LogGroupInput) (LiveTailStream, error) {
	if input.LogGroupARN == "" {
		return nil, errors.New("LogGroupARN is required")
	}
	var pattern *string
	if input.LogEventFilterPattern != "" {
		pattern = &input.LogEventFilterPattern
	}
	var prefixes []string
	if input.LogStreamNamePrefix != "" {
		prefixes = []string{input.LogStreamNamePrefix}
	}

	slti := &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers:   []string{getLogGroupIdentifier(input.LogGroupARN)},
		LogStreamNames:        input.LogStreamNames,
		LogStreamNamePrefixes: prefixes,
		LogEventFilterPattern: pattern,
	}

	slto, err := cwClient.StartLiveTail(ctx, slti)
	if err != nil {
		return nil, err
	}

	return slto.GetStream(), nil
}

type FilterLogEventsAPI interface {
	FilterLogEvents(ctx context.Context, params *cloudwatchlogs.FilterLogEventsInput, optFns ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

func QueryLogGroups(ctx context.Context, cwClient FilterLogEventsAPI, start, end time.Time, limit int32, logGroups ...LogGroupInput) (<-chan LogEvent, <-chan error) {
	var evtsChan <-chan LogEvent
	errChan := make(chan error, len(logGroups))
	var wg sync.WaitGroup
	for _, lgi := range logGroups {
		wg.Add(1)
		lgEvtChan := make(chan LogEvent)
		// Start a go routine for each log group
		go func(lgi LogGroupInput) {
			defer close(lgEvtChan)
			defer wg.Done()
			// CloudWatch only supports querying a LogGroup from a timestamp in
			// ascending order. After we query each LogGroup, we merge the results
			// and take the last N events. Because we can't tell in advance which
			// LogGroup will have the most recent events, we have to query all
			// log groups without limit, and then apply the limit after merging.
			// TODO: optimize this by simulating a descending query by doing
			// multiple queries with time windows, starting from the end time
			// and moving backwards until we have enough events.
			err := QueryLogGroup(ctx, cwClient, lgi, start, end, 0, func(logEvents []LogEvent) error {
				for _, event := range logEvents {
					lgEvtChan <- event
				}
				return nil
			})
			if err != nil {
				errChan <- fmt.Errorf("error querying log group %q: %w", lgi.LogGroupARN, err)
			}
		}(lgi)
		evtsChan = MergeLogEventChan(evtsChan, lgEvtChan) // Merge sort the log events based on timestamp
		if limit > 0 {
			// take the first/last n events only
			if start.IsZero() {
				evtsChan = takeLastN(evtsChan, int(limit))
			} else {
				evtsChan = takeFirstN(evtsChan, int(limit))
			}
		}
	}
	go func() {
		wg.Wait()
		close(errChan)
	}()
	return evtsChan, errChan
}

func QueryLogGroup(ctx context.Context, cwClient FilterLogEventsAPI, input LogGroupInput, start, end time.Time, limit int32, cb func([]LogEvent) error) error {
	return filterLogEvents(ctx, cwClient, input, start, end, limit, cb)
}

func QueryLogGroupStream(ctx context.Context, cwClient FilterLogEventsAPI, input LogGroupInput, start, end time.Time, limit int32) (EventStream[types.StartLiveTailResponseStream], error) {
	ctx, cancel := context.WithCancel(ctx)
	es := newEventStream(cancel) // calling Close on the stream will cancel the context

	go func() {
		defer close(es.ch)
		// TODO: this QueryLogGroup function doesn't return until all logs are fetched, so returning a stream is not very useful
		if err := QueryLogGroup(ctx, cwClient, input, start, end, limit, func(events []LogEvent) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{Value: types.LiveTailSessionUpdate{SessionResults: events}}:
				return nil
			}
		}); err != nil {
			es.err = err
		}
	}()

	return es, nil
}

func filterLogEvents(ctx context.Context, cw FilterLogEventsAPI, lgi LogGroupInput, start, end time.Time, limit int32, cb func([]LogEvent) error) error {
	if lgi.LogGroupARN == "" {
		return errors.New("LogGroupARN is required")
	}
	var pattern *string
	if lgi.LogEventFilterPattern != "" {
		pattern = &lgi.LogEventFilterPattern
	}
	logGroupIdentifier := getLogGroupIdentifier(lgi.LogGroupARN)
	params := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupIdentifier: &logGroupIdentifier,
		LogStreamNames:     lgi.LogStreamNames,
		FilterPattern:      pattern,
	}

	if limit != 0 {
		params.Limit = &limit
	}
	if !start.IsZero() {
		params.StartTime = ptr.Int64(start.UnixMilli())
	}
	if !end.IsZero() {
		params.EndTime = ptr.Int64(end.UnixMilli())
	}
	if start.IsZero() && end.IsZero() {
		// If no time range is specified, limit to the last 60 minutes
		now := time.Now()
		start = now.Add(-60 * time.Minute)
		params.StartTime = ptr.Int64(start.UnixMilli())
		params.EndTime = ptr.Int64(now.UnixMilli())
	}
	if lgi.LogStreamNamePrefix != "" {
		params.LogStreamNamePrefix = &lgi.LogStreamNamePrefix
	}
	for {
		if limit > 0 {
			// Specifying the limit parameter only guarantees that a single page doesn't return more log events than the
			// specified limit, but it might return fewer events than the limit. This is the expected API behavior.
			params.Limit = ptr.Int32(limit)
		}
		fleo, err := cw.FilterLogEvents(ctx, params)
		if err != nil {
			return err
		}
		events := make([]LogEvent, len(fleo.Events))
		for i, event := range fleo.Events {
			events[i] = LogEvent{
				IngestionTime:      event.IngestionTime,
				LogGroupIdentifier: &logGroupIdentifier,
				Message:            event.Message,
				Timestamp:          event.Timestamp,
				LogStreamName:      event.LogStreamName,
			}
		}
		if err := cb(events); err != nil {
			return err
		}
		if fleo.NextToken == nil {
			return nil
		}
		if limit > 0 {
			if len(events) < int(limit) { // this handles len(events) == 0 as well
				limit -= int32(len(events)) // #nosec G115 - always safe because len(events) < limit
			} else if lastTS := events[len(events)-1].Timestamp; lastTS != nil && time.UnixMilli(*lastTS).Equal(start) {
				// If the last event timestamp is equal to the start time, we risk getting stuck in a loop
				// where the agent keeps asking for logs since the last timestamp, but ends up fetching the same logs
				// over and over. To avoid this, we ignore the limit and keep going, until the timestamp changes.
				limit = 10 // arbitrary small number to make some progress; could be smarter
			} else {
				return nil
			}
		}
		params.NextToken = fleo.NextToken
	}
}

func NewCloudWatchLogsClient(ctx context.Context, region aws.Region) (*cloudwatchlogs.Client, error) {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(cfg), nil
}

type LogEvent = types.LiveTailSessionLogEvent

// EventStream is a generic interface that represents a stream of events
type EventStream[T any] interface {
	Events() <-chan T
	Close() error
	Err() error
}

// Deprecated: LiveTailStream is a stream of events from a call to AWS StartLiveTail
type LiveTailStream = EventStream[types.StartLiveTailResponseStream]

func GetLogEvents(e types.StartLiveTailResponseStream) ([]LogEvent, error) {
	switch ev := e.(type) {
	case *types.StartLiveTailResponseStreamMemberSessionStart:
		// fmt.Println("session start:", ev.Value.SessionId)
		return nil, nil // ignore start message
	case *types.StartLiveTailResponseStreamMemberSessionUpdate:
		// fmt.Println("session update:", len(ev.Value.SessionResults))
		return ev.Value.SessionResults, nil
	case nil:
		return nil, io.EOF
	default:
		return nil, fmt.Errorf("unexpected event: %T", ev)
	}
}

func takeLastN[T any](input <-chan T, n int) <-chan T {
	if n <= 0 {
		return input
	}
	out := make(chan T)
	go func() {
		defer close(out)
		var buffer []T
		for evt := range input {
			buffer = append(buffer, evt)
			if len(buffer) > n {
				buffer = buffer[1:] // remove oldest
			}
		}
		for _, evt := range buffer {
			out <- evt
		}
	}()
	return out
}

func takeFirstN[T any](input <-chan T, n int) <-chan T {
	if n <= 0 {
		return input
	}
	out := make(chan T)
	go func() {
		defer close(out)
		count := 0
		for evt := range input {
			out <- evt
			count++
			if count >= n {
				break
			}
		}
	}()
	return out
}
