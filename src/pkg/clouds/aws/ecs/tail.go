package ecs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"path"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

const AwsLogsStreamPrefix = CrunProjectName

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	cwClient, err := cw.NewCloudWatchLogsClient(ctx, a.Region)
	if err != nil {
		return err
	}
	taskId := GetTaskID(taskArn)
	a.Region = region.FromArn(*taskArn)
	tailIter, err := a.TailTaskID(ctx, cwClient, taskId)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	taskch := make(chan error, 1)
	go func() {
		taskch <- WaitForTask(ctx, taskArn, time.Second*3)
		cancel() // stop tailing when task finishes
	}()

	for evt, err := range tailIter {
		if err != nil {
			// If error is due to context cancellation from task completion, return the task result
			if errors.Is(err, context.Canceled) {
				select {
				case taskErr := <-taskch:
					return taskErr
				default:
				}
			}
			return err
		}
		fmt.Println(*evt.Message)
	}
	return <-taskch
}

func (a *AwsEcs) GetTaskArn(taskID string) (TaskArn, error) {
	if taskID == "" {
		return nil, errors.New("taskID is required")
	}
	if a.ClusterName == "" {
		return nil, errors.New("ClusterName is required")
	}
	taskArn := a.MakeARN("ecs", "task/"+a.ClusterName+"/"+taskID)
	return &taskArn, nil
}

func (a *AwsEcs) QueryTaskID(ctx context.Context, cwClient cw.FilterLogEventsAPI, taskID string, start, end time.Time, limit int32) (iter.Seq2[cw.LogEvent, error], error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}

	lgi := cw.LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForTaskID(taskID)}}
	batches, err := cw.QueryLogGroup(ctx, cwClient, lgi, start, end, limit)
	if err != nil {
		return nil, err
	}
	return cw.Flatten(batches), nil
}

func (a *AwsEcs) TailTaskID(ctx context.Context, cwClient cw.StartLiveTailAPI, taskID string) (iter.Seq2[cw.LogEvent, error], error) {
	if taskID == "" {
		return nil, errors.New("taskID is required")
	}
	if a.LogGroupARN == "" {
		return nil, errors.New("LogGroupARN is required")
	}
	if a.ClusterName == "" {
		return nil, errors.New("ClusterName is required")
	}

	lgi := cw.LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForTaskID(taskID)}}
	for {
		logIter, err := cw.TailLogGroup(ctx, cwClient, lgi)
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) {
				return nil, err
			}
			// The log stream doesn't exist yet, so wait for it to be created, but bail out if the task is stopped
			done, err := getTaskStatus(ctx, a.Region, a.ClusterName, taskID)
			if done || err != nil {
				return nil, err // TODO: handle transient errors
			}
			// continue loop, waiting for the log stream to be created; sleep to avoid throttling
			if err := pkg.SleepWithContext(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}
		// TODO: should wrap this iter so we can return io.EOF on task stop
		return logIter, nil
	}
}

func GetCDLogStreamForTaskID(taskID string) string {
	return GetLogStreamForTaskID(CrunProjectName, CdContainerName, taskID)
}

func GetLogStreamForTaskID(awslogsStreamPrefix, containerName, taskID string) string {
	return path.Join(awslogsStreamPrefix, containerName, taskID) // per "awslogs" driver
}

func GetTaskID(taskArn TaskArn) string {
	return path.Base(*taskArn)
}
