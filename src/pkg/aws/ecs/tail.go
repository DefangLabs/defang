package ecs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/defang-io/defang/src/pkg/aws/region"
)

const spinner = `-\|/`

const AwsLogsStreamPrefix = ProjectName

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	taskId := GetTaskID(taskArn)
	a.Region = region.FromArn(*taskArn)
	es, err := a.TailTask(ctx, taskId)
	if err != nil {
		return err
	}

	spinMe := 0
	for {
		err = printLogEvents(ctx, es) // blocking
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) && err != io.EOF {
				return err
			}
			// continue loop, waiting for the log stream to be created
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

func (a *AwsEcs) TailTask(ctx context.Context, taskID string) (EventStream, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}
	logStreamName := getLogStreamForTaskID(taskID)
	return TailLogGroup(ctx, a.LogGroupARN, logStreamName) // TODO: io.EOF on task stop
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
