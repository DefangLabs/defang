package cw

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
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
	FilterLogEventsAPIClient
	StartLiveTailAPI
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

func TailLogGroup(ctx context.Context, cwClient StartLiveTailAPI, input LogGroupInput) (iter.Seq2[[]LogEvent, error], error) {
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

	stream := slto.GetStream()
	return func(yield func([]LogEvent, error) bool) {
		defer stream.Close()
		for {
			select {
			case e := <-stream.Events():
				if err := stream.Err(); err != nil {
					yield(nil, err)
					return
				}
				events, err := getLogEvents(e)
				if err != nil {
					if !yield(nil, err) {
						return
					}
				}
				if !yield(events, nil) {
					return
				}
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			}
		}
	}, nil
}

type FilterLogEventsAPIClient = cloudwatchlogs.FilterLogEventsAPIClient

// Flatten converts an iterator of batches into an iterator of individual items.
func Flatten[T any](seq iter.Seq2[[]T, error]) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for items, err := range seq {
			for _, item := range items {
				if !yield(item, nil) {
					return
				}
			}
			if err != nil {
				var zero T
				if !yield(zero, err) {
					return
				}
			}
		}
	}
}

func QueryLogGroups(ctx context.Context, cwClient FilterLogEventsAPIClient, start, end time.Time, limit int32, logGroups ...LogGroupInput) (iter.Seq2[LogEvent, error], error) {
	var merged iter.Seq2[LogEvent, error]
	for _, lgi := range logGroups {
		logSeq, err := QueryLogGroup(ctx, cwClient, lgi, start, end, limit)
		if err != nil {
			return nil, err
		}
		merged = MergeLogEvents(merged, Flatten(logSeq)) // Merge sort the log events based on timestamp
		if limit > 0 {
			// take the first/last n events only from the merged stream
			if start.IsZero() {
				merged = TakeLastN(merged, int(limit))
			} else {
				merged = TakeFirstN(merged, int(limit))
			}
		}
	}
	return merged, nil
}

func QueryLogGroup(ctx context.Context, cw FilterLogEventsAPIClient, lgi LogGroupInput, start, end time.Time, limit int32) (iter.Seq2[[]LogEvent, error], error) {
	if lgi.LogGroupARN == "" {
		return nil, errors.New("LogGroupARN is required")
	}
	var pattern *string
	if lgi.LogEventFilterPattern != "" {
		pattern = &lgi.LogEventFilterPattern
	}
	logGroupIdentifier := getLogGroupIdentifier(lgi.LogGroupARN)
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		// CloudWatch only supports querying a LogGroup from a timestamp in
		// ascending order. After we query each LogGroup, we merge the results
		// and take the last N events. Because we can't tell in advance which
		// LogGroup will have the most recent events, we have to query all
		// log groups without limit, and then apply the limit after merging.
		// TODO: optimize this by simulating a descending query by doing
		// multiple queries with time windows, starting from the end time
		// and moving backwards until we have enough events.
		start = end.Add(-60 * time.Minute)
		limit = 0
	}

	params := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupIdentifier: &logGroupIdentifier,
		LogStreamNames:     lgi.LogStreamNames,
		FilterPattern:      pattern,
		StartTime:          ptr.Int64(start.UnixMilli()),   // rounds down
		EndTime:            ptr.Int64(end.UnixMilli() + 1), // round up
	}
	if lgi.LogStreamNamePrefix != "" {
		params.LogStreamNamePrefix = &lgi.LogStreamNamePrefix
	}
	return func(yield func([]LogEvent, error) bool) {
		for {
			if limit > 0 {
				// Specifying the limit parameter only guarantees that a single page doesn't return more log events than the
				// specified limit, but it might return fewer events than the limit. This is the expected API behavior.
				params.Limit = ptr.Int32(limit)
			}
			fleo, err := cw.FilterLogEvents(ctx, params)
			if err != nil {
				yield(nil, err)
				return
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
			if !yield(events, nil) {
				return
			}
			if fleo.NextToken == nil {
				return
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
					return
				}
			}
			params.NextToken = fleo.NextToken
		}
	}, nil
}

func NewCloudWatchLogsClient(ctx context.Context, region aws.Region) (*cloudwatchlogs.Client, error) {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, err
	}
	return cloudwatchlogs.NewFromConfig(cfg), nil
}

type LogEvent = types.LiveTailSessionLogEvent

func getLogEvents(e types.StartLiveTailResponseStream) ([]LogEvent, error) {
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
