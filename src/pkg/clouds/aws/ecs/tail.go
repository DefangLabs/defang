package ecs

import (
	"context"
	"errors"
	"fmt"
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
	es, err := a.TailTaskID(ctx, cwClient, taskId)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	taskch := make(chan error)
	defer close(taskch)
	go func() {
		taskch <- WaitForTask(ctx, taskArn, time.Second*3)
	}()

	for {
		select {
		case e := <-es.Events(): // blocking
			events, err := cw.GetLogEvents(e)
			// Print before checking for errors, so we don't lose any logs in case of EOF
			for _, event := range events {
				fmt.Println(*event.Message)
			}
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case err := <-taskch:
			return err
		}
	}
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

func (a *AwsEcs) QueryTaskID(ctx context.Context, cwClient cw.FilterLogEventsAPI, taskID string, start, end time.Time, limit int32) (cw.LiveTailStream, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}

	lgi := cw.LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForTaskID(taskID)}}
	return cw.QueryLogGroupStream(ctx, cwClient, lgi, start, end, limit)
}

func (a *AwsEcs) TailTaskID(ctx context.Context, cwClient cw.StartLiveTailAPI, taskID string) (cw.LiveTailStream, error) {
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
		stream, err := cw.TailLogGroup(ctx, cwClient, lgi)
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
			if err := pkg.SleepWithContext(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}
		// TODO: should wrap this stream so we can return io.EOF on task stop
		return stream, nil
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
