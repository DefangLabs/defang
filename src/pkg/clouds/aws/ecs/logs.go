package ecs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/DefangLabs/defang/src/pkg/term"
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

func NewStaticLogStream(ch <-chan LogEvent, cancel func()) EventStream[types.StartLiveTailResponseStream] {
	es := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

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

func QueryAndTailLogGroups(ctx context.Context, start, end time.Time, follow bool, logGroups ...LogGroupInput) (LiveTailStream, error) {
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
		es, err = QueryAndTailLogGroup(ctx, lgi, start, end, follow)
		if err != nil {
			break // abort if there is any fatal error
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

func TailLogGroup(ctx context.Context, input LogGroupInput) (LiveTailStream, error) {
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

	region := region.FromArn(slti.LogGroupIdentifiers[0]) // must have at least one log group
	cw, err := newCloudWatchLogsClient(ctx, region)       // assume all log groups are in the same region
	if err != nil {
		return nil, err
	}

	slto, err := cw.StartLiveTail(ctx, slti)
	if err != nil {
		return nil, err
	}

	return slto.GetStream(), nil
}

func QueryLogGroups(ctx context.Context, start, end time.Time, limit int, logGroups ...LogGroupInput) (<-chan LogEvent, <-chan error) {
	// Gather logs from the CD task, kaniko, ECS events, and all services
	var evtsChan chan LogEvent
	for _, lgi := range logGroups {
		lgEvtChan := make(chan LogEvent)
		// Start a go routine for each log group
		go func(lgi LogGroupInput) {
			defer close(lgEvtChan)
			if err := QueryLogGroup(ctx, lgi, start, end, func(logEvents []LogEvent) error {
				for _, event := range logEvents {
					lgEvtChan <- event
				}
				return nil
			}); err != nil {
				term.Errorf("error querying log group %s: %v", lgi.LogGroupARN, err)
			}
		}(lgi)
		evtsChan = mergeLogEventChan(evtsChan, lgEvtChan) // Merge sort the log events based on timestamp
		// take the last n events only
		if limit > 0 {
			evtsChan = takeLastN(evtsChan, limit)
		}
	}
	return evtsChan, nil
}

func QueryLogGroup(ctx context.Context, input LogGroupInput, start, end time.Time, cb func([]LogEvent) error) error {
	region := region.FromArn(input.LogGroupARN)
	cw, err := newCloudWatchLogsClient(ctx, region)
	if err != nil {
		return err
	}
	return filterLogEvents(ctx, cw, input, start, end, cb)
}

func filterLogEvents(ctx context.Context, cw *cloudwatchlogs.Client, lgi LogGroupInput, start, end time.Time, cb func([]LogEvent) error) error {
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
		params.NextToken = fleo.NextToken
	}
}

func newCloudWatchLogsClient(ctx context.Context, region aws.Region) (*cloudwatchlogs.Client, error) {
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

func takeLastN[T any](input chan T, n int) chan T {
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
