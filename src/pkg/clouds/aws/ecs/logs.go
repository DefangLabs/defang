package ecs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
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

func QueryAndTailLogGroups(ctx context.Context, start, end time.Time, logGroups ...LogGroupInput) (LiveTailStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	errCh := make(chan error)

	var eventCh chan LogEvent
	for _, lgi := range logGroups {
		es, err := QueryAndTailLogGroup(ctx, lgi, start, end)
		if err != nil {
			cancel()
			return nil, err
		}
		newCh := LiveTailStreamToChannel(ctx, es, errCh)
		eventCh = mergeLogEventChan(eventCh, newCh)
	}

	e := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	go func() {
		defer close(e.ch)
		for {
			select {
			case event := <-eventCh:
				e.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
					Value: types.LiveTailSessionUpdate{SessionResults: []types.LiveTailSessionLogEvent{event}},
				}
			case err := <-errCh:
				e.err = err
				return // defered close of e.ch will unblock the caller to pick up the error
			case <-ctx.Done():
				return
			}
		}
	}()
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

func QueryLogGroups(ctx context.Context, start, end time.Time, logGroups ...LogGroupInput) (<-chan LogEvent, <-chan error) {
	errsChan := make(chan error)

	// Gather logs from the CD task, kaniko, ECS events, and all services
	var evtsChan chan LogEvent
	for _, lgi := range logGroups {
		lgEvtChan := make(chan LogEvent)
		// Start a go routine for each log group
		go func(lgi LogGroupInput) {
			defer close(errsChan)
			if err := QueryLogGroup(ctx, lgi, start, end, func(logEvents []LogEvent) error {
				for _, event := range logEvents {
					lgEvtChan <- event
				}
				return nil
			}); err != nil {
				errsChan <- err
			}
		}(lgi)
		evtsChan = mergeLogEventChan(evtsChan, lgEvtChan) // Merge sort the log events based on timestamp
	}
	return evtsChan, errsChan
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
		StartTime:          ptr.Int64(start.UnixMilli()),
		EndTime:            ptr.Int64(end.UnixMilli()),
		LogGroupIdentifier: &logGroupIdentifier,
		LogStreamNames:     lgi.LogStreamNames,
		FilterPattern:      pattern,
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
