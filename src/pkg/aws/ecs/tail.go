package ecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
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

func (a *AwsEcs) TailTask(ctx context.Context, taskArn TaskArn) (*LogStreamer, error) {
	logStreamName := getLogStreamForTask(taskArn)
	return a.TailLogStream(ctx, logStreamName) // TODO: io.EOF on task stop
}

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	a.Region = region.FromArn(string(*taskArn))
	s, err := a.TailTask(ctx, taskArn)
	if err != nil {
		return err
	}

	taskId := taskID(taskArn)
	spinMe := 0
	for {
		err = s.printLogEvents(ctx)
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
			s.printLogEvents(ctx)
			return err
		}

		fmt.Printf("%c\r", spinner[spinMe%len(spinner)])
		spinMe++
		pkg.SleepWithContext(ctx, time.Second)
	}
}

func (a *AwsEcs) TailLogStream(ctx context.Context, logStreamNamePrefixes ...string) (*LogStreamer, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	cw := cloudwatchlogs.NewFromConfig(cfg)
	s, err := cw.StartLiveTail(ctx, &cloudwatchlogs.StartLiveTailInput{
		// LogEventFilterPattern: aws.String(""),
		LogGroupIdentifiers: []string{strings.TrimSuffix(a.LogGroupARN, ":*")},
		// LogStreamNamePrefixes: logStreamNamePrefixes,
		// LogStreamNames: logStreamNamePrefixes,
	})
	if err != nil {
		return nil, err
	}

	return &LogStreamer{
		StartLiveTailEventStream: s.GetStream(),
	}, nil
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

type LogEntry struct {
	Message   string
	Timestamp time.Time
	// Stderr    bool
}

type LogStreamer struct {
	*cloudwatchlogs.StartLiveTailEventStream
}

func (s LogStreamer) Close() error {
	return s.StartLiveTailEventStream.Close()
}

func (s LogStreamer) Receive(ctx context.Context) ([]LogEntry, error) {
	select {
	case e := <-s.Events(): // blocking
		switch ev := e.(type) {
		case *types.StartLiveTailResponseStreamMemberSessionStart:
			// fmt.Println("session start:", ev.Value.SessionId)
			return nil, nil // ignore start message
		case *types.StartLiveTailResponseStreamMemberSessionUpdate:
			// fmt.Println("session update:", len(ev.Value.SessionResults))
			entries := make([]LogEntry, len(ev.Value.SessionResults))
			for i, event := range ev.Value.SessionResults {
				entries[i] = LogEntry{
					Message:   *event.Message, // TODO: parse JSON if this is from awsfirelens
					Timestamp: time.UnixMilli(*event.Timestamp),
					// Server: LogStreamName,
				}
			}
			return entries, nil
		default:
			return nil, fmt.Errorf("unexpected event: %T", ev)
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *LogStreamer) printLogEvents(ctx context.Context) error {
	for {
		events, err := s.Receive(ctx)
		for _, event := range events {
			fmt.Println(event.Message)
		}
		if err != nil {
			return err
		}
	}
}
