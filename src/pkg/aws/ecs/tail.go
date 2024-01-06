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
	return a.TailLogStream(ctx, logStreamName)
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
		time.Sleep(1 * time.Second)
	}
}

func (a *AwsEcs) TailLogStream(ctx context.Context, logStreamName string) (*LogStreamer, error) {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}

	return &LogStreamer{
		logGroupName:  a.LogGroupName,
		logStreamName: logStreamName,
		cw:            cloudwatchlogs.NewFromConfig(cfg),
	}, nil
}

func (a *AwsEcs) taskStatus(ctx context.Context, taskId string) error {
	cfg, _ := a.LoadConfig(ctx)
	ecsClient := ecs.NewFromConfig(cfg)

	// Use DescribeTasks API to check if the task is still running (same as ecs.NewTasksStoppedWaiter)
	ti, _ := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(a.ClusterARN), // clusterArnFromTaskArn
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
	logGroupName  string
	logStreamName string
	cw            *cloudwatchlogs.Client
	nextToken     *string
}

func (s *LogStreamer) Receive(ctx context.Context) ([]LogEntry, error) {
	events, err := s.cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &s.logGroupName,
		LogStreamName: &s.logStreamName,
		NextToken:     s.nextToken,
		StartFromHead: aws.Bool(true),
	})
	if err != nil {
		return nil, err
	}
	entries := make([]LogEntry, len(events.Events))
	for i, event := range events.Events {
		entries[i] = LogEntry{
			Message:   *event.Message, // TODO: parse JSON if this is from awsfirelens
			Timestamp: time.UnixMilli(*event.Timestamp),
		}
	}
	if s.nextToken != nil && *s.nextToken == *events.NextForwardToken {
		return entries, io.EOF // no new logs
	}
	s.nextToken = events.NextForwardToken
	return entries, nil
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
