package ecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/smithy-go/ptr"
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

func TailLogGroups(ctx context.Context, since time.Time, logGroups ...LogGroupInput) (EventStream, error) {
	cs := newCollectionStream(ctx)

	type pair struct {
		es  EventStream
		lgi LogGroupInput
	}
	var pairs []pair
	var pendingGroups []LogGroupInput

	sincePending := since
	if sincePending.Year() <= 1970 {
		sincePending = time.Now()
	}
	for _, lgi := range logGroups {
		es, err := TailLogGroup(ctx, lgi)
		if err == nil {
			pairs = append(pairs, pair{es, lgi})
			continue
		}

		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		pendingGroups = append(pendingGroups, lgi)
	}

	// Start goroutines to wait for the log group to be created for the resource not found log groups
	for _, lgi := range pendingGroups {
		cs.wg.Add(1)
		go func(lgi LogGroupInput) {
			defer cs.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-cs.ctx.Done():
					return
				case <-ticker.C:
					es, err := TailLogGroup(cs.ctx, lgi)
					if err == nil {
						cs.addAndStart(es, sincePending, lgi)
						return
					}
					var resourceNotFound *types.ResourceNotFoundException
					if !errors.As(err, &resourceNotFound) {
						cs.errCh <- err
						return
					}
				}
			}
		}(lgi)
	}

	// Only add and start watching the streams if there were no errors, prevent lingering goroutines
	for _, s := range pairs {
		cs.addAndStart(s.es, since, s.lgi)
	}

	return cs, nil
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
	return startLiveTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		LogGroupIdentifiers:   []string{getLogGroupIdentifier(input.LogGroupARN)},
		LogStreamNames:        input.LogStreamNames,
		LogStreamNamePrefixes: prefixes,
		LogEventFilterPattern: pattern,
	})
}

func Query(ctx context.Context, input LogGroupInput, start, end time.Time, cb func([]LogEvent)) error {
	region := region.FromArn(input.LogGroupARN)
	cw, err := newCloudWatchLogsClient(ctx, region)
	if err != nil {
		return err
	}
	return filterLogEvents(ctx, cw, input, start, end, cb)
}

func filterLogEvents(ctx context.Context, cw *cloudwatchlogs.Client, lgi LogGroupInput, start, end time.Time, cb func([]LogEvent)) error {
	logGroupIdentifier := getLogGroupIdentifier(lgi.LogGroupARN)
	params := &cloudwatchlogs.FilterLogEventsInput{
		StartTime:          ptr.Int64(start.UnixMilli()),
		EndTime:            ptr.Int64(end.UnixMilli()),
		LogGroupIdentifier: &logGroupIdentifier,
		LogStreamNames:     lgi.LogStreamNames,
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
		cb(events)
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

func startLiveTail(ctx context.Context, slti *cloudwatchlogs.StartLiveTailInput) (EventStream, error) {
	region := region.FromArn(slti.LogGroupIdentifiers[0]) // must have at least one log group
	cw, err := newCloudWatchLogsClient(ctx, region)
	if err != nil {
		return nil, err
	}

	slto, err := cw.StartLiveTail(ctx, slti)
	if err != nil {
		return nil, err
	}

	return slto.GetStream(), nil
}

func GetTaskStatus(ctx context.Context, taskArn TaskArn) error {
	region := region.FromArn(*taskArn)
	cluster, taskID := SplitClusterTask(taskArn)
	return getTaskStatus(ctx, region, cluster, taskID)
}

func isTaskTerminalStatus(status string) bool {
	// From https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-lifecycle-explanation.html
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
	if task.LastStatus == nil || !isTaskTerminalStatus(*task.LastStatus) {
		return nil // still running
	}

	switch task.StopCode {
	default:
		return TaskFailure{task.StopCode, *task.StoppedReason}
	case ecsTypes.TaskStopCodeEssentialContainerExited:
		for _, c := range task.Containers {
			if c.ExitCode != nil && *c.ExitCode != 0 {
				reason := fmt.Sprintf("%s with code %d", *task.StoppedReason, *c.ExitCode)
				return TaskFailure{task.StopCode, reason}
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
	Err() error
}

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
		if since.Year() > 1970 {
			// Query the logs between the start time and now
			if err := Query(c.ctx, lgi, since, time.Now(), func(events []LogEvent) {
				c.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
					Value: types.LiveTailSessionUpdate{SessionResults: events},
				}
			}); err != nil {
				c.errCh <- err // the caller will likely cancel the context
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
