package ecs

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/aws/region"
)

const spinner = `-\|/`

const AwsLogsStreamPrefix = ProjectName

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	taskId := GetTaskID(taskArn)
	a.Region = region.FromArn(*taskArn)
	es, err := a.TailTaskID(ctx, taskId)
	if err != nil {
		return err
	}

	spinMe := 0
	for {
		err = printLogEvents(ctx, es) // blocking
		if err != nil {
			return err
		}

		err := getTaskStatus(ctx, a.Region, a.ClusterName, taskId)
		if err != nil {
			// Before we exit, print any remaining logs (ignore errors)
			printLogEvents(ctx, es)
			return err
		}

		fmt.Printf("%c\r", spinner[spinMe%len(spinner)])
		spinMe++
	}
}

func (a *AwsEcs) GetTaskArn(taskID string) (TaskArn, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}
	taskArn := a.MakeARN("ecs", "task/"+a.ClusterName+"/"+taskID)
	return &taskArn, nil
}

func (a *AwsEcs) TailTaskID(ctx context.Context, taskID string) (EventStream, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}
	logStreamName := getLogStreamForTaskID(taskID)
	for {
		stream, err := TailLogGroup(ctx, a.LogGroupARN, logStreamName)
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) {
				return nil, err
			}
			// The log stream doesn't exist yet, so wait for it to be created, but bail out if the task is stopped
			err := getTaskStatus(ctx, a.Region, a.ClusterName, taskID)
			if err != nil {
				return nil, err
			}
			// continue loop, waiting for the log stream to be created; sleep to avoid throttling
			pkg.SleepWithContext(ctx, time.Second)
			continue
		}
		// TODO: should wrap this stream so we can return io.EOF on task stop
		return stream, nil
	}
}

func getLogStreamForTaskID(taskID string) string {
	return path.Join(AwsLogsStreamPrefix, ContainerName, taskID) // per "awslogs" driver
}

func GetTaskID(taskArn TaskArn) string {
	return path.Base(*taskArn)
}

func printLogEvents(ctx context.Context, es EventStream) error {
	for {
		select {
		case e := <-es.Events(): // blocking
			events, err := GetLogEvents(e)
			// Print before checking for errors, so we don't lose any logs in case of EOF
			for _, event := range events {
				fmt.Println(*event.Message)
			}
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
