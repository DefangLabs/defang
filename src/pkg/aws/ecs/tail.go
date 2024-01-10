package ecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws/region"
)

const spinner = `-\|/`

const AwsLogsStreamPrefix = ProjectName

func getLogStreamForTask(taskArn TaskArn) string {
	return path.Join(AwsLogsStreamPrefix, ContainerName, taskID(taskArn)) // per "awslogs" driver
}

func taskID(taskArn TaskArn) string {
	return path.Base(*taskArn)
}

func (a *AwsEcs) TailTask(ctx context.Context, taskArn TaskArn) (EventStream, error) {
	logStreamName := getLogStreamForTask(taskArn)
	return a.TailLogStreams(ctx, a.LogGroupARN, logStreamName) // TODO: io.EOF on task stop
}

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	a.Region = region.FromArn(string(*taskArn))
	es, err := a.TailTask(ctx, taskArn)
	if err != nil {
		return err
	}

	taskId := taskID(taskArn)
	spinMe := 0
	for {
		err = printLogEvents(ctx, es)
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) && err != io.EOF {
				return err
			}
			// continue loop, waiting for the log stream to be created
		}

		err := a.taskStatus(ctx, taskId)
		if err != nil {
			// Before we exit, print any remaining logs
			printLogEvents(ctx, es)
			return err
		}

		fmt.Printf("%c\r", spinner[spinMe%len(spinner)])
		spinMe++
		pkg.SleepWithContext(ctx, time.Second)
	}
}

func getLogGroupIdentifier(arnOrId string) string {
	return strings.TrimSuffix(arnOrId, ":*")
}

func (a *AwsEcs) TailLogGroups(ctx context.Context, logGroups ...string) (EventStream, error) {
	var cs = collectionStream{
		ch:    make(chan types.StartLiveTailResponseStream),
		errCh: make(chan error),
		done:  make(chan struct{}),
	}

	var streams []EventStream
	var waitingStreamIDs []string

	for _, lg := range logGroups {
		lgID := getLogGroupIdentifier(lg)
		es, _, err := a.startTail(ctx, &cloudwatchlogs.StartLiveTailInput{LogGroupIdentifiers: []string{lgID}})
		if err == nil {
			streams = append(streams, es)
			continue
		}

		var resourceNotFound *types.ResourceNotFoundException
		if !errors.As(err, &resourceNotFound) {
			return nil, err
		}
		waitingStreamIDs = append(waitingStreamIDs, lgID)
	}

	// Start goroutines to wait for the log group to be created for the resource not found log groups
	for _, lgID := range waitingStreamIDs {
		cs.wg.Add(1)
		go func(ctx context.Context, lgID string) {
			defer cs.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-cs.done:
					return
				case <-ticker.C:
					es, _, err := a.startTail(ctx, &cloudwatchlogs.StartLiveTailInput{LogGroupIdentifiers: []string{lgID}})
					if err == nil {
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

		}(ctx, lgID)
	}

	// Only add and start watching the streams if there were no errors, prevent lingering goroutines
	for _, s := range streams {
		cs.addAndStart(s)
	}

	return &cs, nil
}

func (a *AwsEcs) TailLogStreams(ctx context.Context, logGroupArn string, logStreams ...string) (EventStream, error) {
	logGroupIDs := []string{getLogGroupIdentifier(logGroupArn)}
	es, _, err := a.startTail(ctx, &cloudwatchlogs.StartLiveTailInput{LogGroupIdentifiers: logGroupIDs, LogStreamNames: logStreams})
	if err != nil {
		return nil, err
	}
	return es, nil
}

func (a *AwsEcs) startTail(ctx context.Context, slti *cloudwatchlogs.StartLiveTailInput) (EventStream, *cloudwatchlogs.Client, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, nil, err
	}

	cw := cloudwatchlogs.NewFromConfig(cfg)
	slto, err := cw.StartLiveTail(ctx, slti)
	if err != nil {
		// var resourceNotFound *types.ResourceNotFoundException
		// if errors.As(err, &resourceNotFound) {
		// 	return nil, fmt.Errorf("log group not found: %w", err)
		// }
		return nil, nil, err
	}

	return slto.GetStream(), cw, nil
}

func (a *AwsEcs) taskStatus(ctx context.Context, taskId string) error {
	cfg, _ := a.LoadConfig(ctx)
	ecsClient := ecs.NewFromConfig(cfg)

	// Use DescribeTasks API to check if the task is still running (same as ecs.NewTasksStoppedWaiter)
	ti, _ := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(a.ClusterName), // clusterArnFromTaskArn
		Tasks:   []string{taskId},
	})
	if ti != nil && len(ti.Tasks) > 0 {
		task := ti.Tasks[0]
		switch task.StopCode {
		default:
			// Before we exit, grab any remaining logs
			return taskFailure{string(task.StopCode), *task.StoppedReason}
		case ecsTypes.TaskStopCodeEssentialContainerExited:
			// Before we exit, grab any remaining logs
			if *task.Containers[0].ExitCode == 0 {
				return io.EOF // Success
			}
			reason := fmt.Sprintf("%s with code %d", *task.StoppedReason, *task.Containers[0].ExitCode)
			return taskFailure{string(task.StopCode), reason}
		case "": // Task is still running
		}
	}
	return nil
}

func clusterArnFromTaskArn(taskArn string) string {
	arnParts := strings.Split(taskArn, ":")
	if len(arnParts) != 6 {
		panic("invalid task ARN")
	}
	resourceParts := strings.Split(arnParts[5], "/")
	if len(resourceParts) != 3 || resourceParts[0] != "task" {
		panic("invalid task ARN")
	}
	return fmt.Sprintf("arn:aws:ecs:%s:%s:cluster/%s", arnParts[3], arnParts[4], resourceParts[1])
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
			case e := <-s.Events():
				select {
				case c.ch <- e:
				case <-c.done:
					return
				}
			case <-c.done:
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

func (c *collectionStream) Err() <-chan error {
	return c.errCh
}

func printLogEvents(ctx context.Context, es EventStream) error {
	for {
		select {
		case e := <-es.Events(): // blocking
			events, err := ConvertEvents(e)
			if err != nil {
				return err
			}
			for _, event := range events {
				fmt.Println(*event.Message)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func ConvertEvents(e types.StartLiveTailResponseStream) ([]LogEvent, error) {
	switch ev := e.(type) {
	case *types.StartLiveTailResponseStreamMemberSessionStart:
		// fmt.Println("session start:", ev.Value.SessionId)
		return nil, nil // ignore start message
	case *types.StartLiveTailResponseStreamMemberSessionUpdate:
		// fmt.Println("session update:", len(ev.Value.SessionResults))
		return ev.Value.SessionResults, nil
	default:
		return nil, fmt.Errorf("unexpected event: %T", ev)
	}
}
