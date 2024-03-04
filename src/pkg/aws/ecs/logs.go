package ecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go/ptr"
	"github.com/defang-io/defang/src/pkg/aws"
	"github.com/defang-io/defang/src/pkg/aws/region"
)

// Task ARN						arn:aws:ecs:us-west-2:123456789012:task/CLUSTER_NAME/2cba912d5eb14ffd926f6992b054f3bf
// Cluster ARN					arn:aws:ecs:us-west-2:123456789012:cluster/CLUSTER_NAME
// LogGroup ARN					arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME:*
// LogGroup ID					arn:aws:logs:us-west-2:123456789012:log-group:/LOG/GROUP/NAME
// LogStream ("awslogs")		PREFIX/CONTAINER/2cba912d5eb14ffd926f6992b054f3bf
// LogStream ("awsfirelens")	PREFIX/CONTAINER-firelens-2cba912d5eb14ffd926f6992b054f3bf

type LogStreamInfo struct {
	Prefix    string
	Container string
	Firelens  bool
	TaskID    string
}

func GetLogStreamInfo(logStream string) *LogStreamInfo {
	parts := strings.Split(logStream, "/")
	switch len(parts) {
	case 3:
		return &LogStreamInfo{
			Prefix:    parts[0],
			Container: parts[1],
			Firelens:  false,
			TaskID:    parts[2],
		}
	case 2:
		firelensParts := strings.Split(parts[1], "-")
		if len(firelensParts) != 3 || firelensParts[1] != "firelens" {
			return nil
		}
		return &LogStreamInfo{
			Prefix:    parts[0],
			Container: firelensParts[0],
			Firelens:  true,
			TaskID:    firelensParts[2],
		}
	default:
		return nil
	}
}

func getLogGroupIdentifier(arnOrId string) string {
	return strings.TrimSuffix(arnOrId, ":*")
}

func TailLogGroups(ctx context.Context, logGroups ...LogGroupInput) (EventStream, error) {
	var cs = collectionStream{
		ch:    make(chan types.StartLiveTailResponseStream),
		errCh: make(chan error),
		done:  make(chan struct{}),
	}

	var streams []EventStream
	var pendingGroups []LogGroupInput

	for _, lg := range logGroups {
		es, err := TailLogGroup(ctx, lg)
		if err == nil {
			streams = append(streams, es)
			continue
		}

		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		pendingGroups = append(pendingGroups, lg)
	}

	// Start goroutines to wait for the log group to be created for the resource not found log groups
	since := time.Now()
	for _, lgi := range pendingGroups {
		cs.wg.Add(1)
		go func(ctx context.Context, lgi LogGroupInput) {
			defer cs.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-cs.done:
					return
				case <-ticker.C:
					es, err := TailLogGroup(ctx, lgi)
					if err == nil {
						// Query the logs between the start time and now
						if events, err := Query(ctx, lgi, since, time.Now()); err == nil {
							// println("found logs:", len(events))
							cs.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
								Value: types.LiveTailSessionUpdate{SessionResults: events},
							}
						} else {
							// println("error querying logs:", err)
						}
						cs.addAndStart(es)
						return
					}
					var resourceNotFound *types.ResourceNotFoundException
					if !errors.As(err, &resourceNotFound) {
						cs.errCh <- err
						return
					}
				}
			}
		}(ctx, lgi)
	}

	// Only add and start watching the streams if there were no errors, prevent lingering goroutines
	for _, s := range streams {
		cs.addAndStart(s)
	}

	return &cs, nil
}

// LogGroupInput is like cloudwatchlogs.StartLiveTailInput but with only one loggroup and one logstream prefix.
type LogGroupInput struct {
	LogGroupARN           string
	LogStreamNames        []string
	LogStreamNamePrefix   string
	LogEventFilterPattern string
}

func TailLogGroup(ctx context.Context, input LogGroupInput) (EventStream, error) {
	var pattern *string
	if input.LogEventFilterPattern != "" {
		pattern = &input.LogEventFilterPattern
	}
	var prefixes []string
	if input.LogStreamNamePrefix != "" {
		prefixes = []string{input.LogStreamNamePrefix}
	}
	return startTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers:   []string{getLogGroupIdentifier(input.LogGroupARN)},
		LogStreamNames:        input.LogStreamNames,
		LogStreamNamePrefixes: prefixes,
		LogEventFilterPattern: pattern,
	})
}

func Query(ctx context.Context, lgi LogGroupInput, start time.Time, end time.Time) ([]LogEvent, error) {
	region := region.FromArn(lgi.LogGroupARN)
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, err
	}

	logGroupIdentifier := getLogGroupIdentifier(lgi.LogGroupARN)
	var prefix *string
	if lgi.LogStreamNamePrefix != "" {
		prefix = &lgi.LogStreamNamePrefix
	}
	cw := cloudwatchlogs.NewFromConfig(cfg)
	fleo, err := cw.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{
		StartTime:           ptr.Int64(start.UnixMilli()),
		EndTime:             ptr.Int64(end.UnixMilli()),
		LogGroupIdentifier:  &logGroupIdentifier,
		LogStreamNamePrefix: prefix,
		LogStreamNames:      lgi.LogStreamNames,
	})
	if err != nil {
		return nil, err
	}
	events := make([]LogEvent, len(fleo.Events))
	for i, e := range fleo.Events {
		events[i] = LogEvent{
			IngestionTime:      e.IngestionTime,
			LogGroupIdentifier: &logGroupIdentifier,
			Message:            e.Message,
			Timestamp:          e.Timestamp,
			LogStreamName:      e.LogStreamName,
		}
	}
	// TODO: handle pagination using NextToken
	return events, nil
}

func startTail(ctx context.Context, slti *cloudwatchlogs.StartLiveTailInput) (EventStream, error) {
	region := region.FromArn(slti.LogGroupIdentifiers[0]) // must have at least one log group
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return nil, err
	}

	cw := cloudwatchlogs.NewFromConfig(cfg)
	slto, err := cw.StartLiveTail(ctx, slti)
	if err != nil {
		return nil, err
	}

	// if !since.IsZero() {
	// 	if events, err := Query(ctx, slti.LogGroupIdentifiers[0], since, time.Now()); err == nil {
	// 		slto.Events <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
	// 			Value: types.LiveTailSessionUpdate{
	// 				SessionResults: events,
	// 			},
	// 		}
	// 	}
	// }

	return slto.GetStream(), nil
}

func GetTaskStatus(ctx context.Context, taskArn TaskArn) error {
	region := region.FromArn(*taskArn)
	cluster, taskID := SplitClusterTask(taskArn)
	return getTaskStatus(ctx, region, cluster, taskID)
}

func isTaskTerminalState(status string) bool {
	// println("status:", status)
	// From https://docs.aws.amazon.com/AmazonECS/latest/developerguide/scheduling_tasks.html#task-lifecycle
	switch status {
	case "DELETED", "STOPPED", "DEPROVISIONING":
		return true
	default:
		return false // we might still get logs
	}
}

func getTaskStatus(ctx context.Context, region aws.Region, cluster, taskId string) error {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	// Use DescribeTasks API to check if the task is still running (same as ecs.NewTasksStoppedWaiter)
	ti, _ := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: &cluster,
		Tasks:   []string{taskId},
	})
	if ti == nil || len(ti.Tasks) == 0 {
		return nil // task doesn't exist yet; TODO: check the actual error from DescribeTasks
	}
	task := ti.Tasks[0]
	if task.LastStatus == nil || !isTaskTerminalState(*task.LastStatus) {
		return nil // still running
	}

	switch task.StopCode {
	default:
		return taskFailure{string(task.StopCode), *task.StoppedReason}
	case ecsTypes.TaskStopCodeEssentialContainerExited:
		for _, c := range task.Containers {
			if c.ExitCode != nil && *c.ExitCode != 0 {
				reason := fmt.Sprintf("%s with code %d", *task.StoppedReason, *c.ExitCode)
				return taskFailure{string(task.StopCode), reason}
			}
		}
		fallthrough
	case "": // TODO: shouldn't happen
		return io.EOF // Success
	}
}

func SplitClusterTask(taskArn TaskArn) (string, string) {
	if !strings.HasPrefix(*taskArn, "arn:aws:ecs:") {
		panic("invalid ECS ARN")
	}
	parts := strings.Split(*taskArn, "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[0], ":task") {
		panic("invalid task ARN")
	}
	return parts[1], parts[2]
}

type LogEvent = types.LiveTailSessionLogEvent

// EventStream is an interface that represents a stream of events from a call to StartLiveTail
type EventStream interface {
	Close() error
	Events() <-chan types.StartLiveTailResponseStream
}

type collectionStream struct {
	streams []EventStream
	ch      chan types.StartLiveTailResponseStream
	errCh   chan error

	done chan struct{}
	lock sync.Mutex
	wg   sync.WaitGroup
}

func (c *collectionStream) addAndStart(s EventStream) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.streams = append(c.streams, s)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			// Double select to make sure context cancellation is not blocked by either the receive or send
			// See: https://stackoverflow.com/questions/60030756/what-does-it-mean-when-one-channel-uses-two-arrows-to-write-to-another-channel
			select {
			case e := <-s.Events(): // blocking
				select {
				case c.ch <- e:
				case <-c.done:
					return
				}
			case <-c.done: // blocking
				return
			}
		}
	}()
}

func (c *collectionStream) Close() error {
	close(c.done)
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
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *collectionStream) Events() <-chan types.StartLiveTailResponseStream {
	return c.ch
}

func (c *collectionStream) Errs() <-chan error {
	return c.errCh
}

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

func WaitForTask(ctx context.Context, taskArn TaskArn, poll time.Duration) error {
	if taskArn == nil {
		panic("taskArn is nil")
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Handle cancellation
			return ctx.Err()
		case <-ticker.C:
			if err := GetTaskStatus(ctx, taskArn); err != nil {
				return err
			}
		}
	}
}
