package ecs

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/region"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

const AwsLogsStreamPrefix = CrunProjectName

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	taskId := GetTaskID(taskArn)
	a.Region = region.FromArn(*taskArn)
	es, err := a.TailTaskID(ctx, taskId)
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
		case err := <-taskch:
			return err
		}
	}
}

func (a *AwsEcs) GetTaskArn(taskID string) (TaskArn, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}
	taskArn := a.MakeARN("ecs", "task/"+a.ClusterName+"/"+taskID)
	return &taskArn, nil
}

func (a *AwsEcs) QueryTaskID(ctx context.Context, taskID string, start, end time.Time, limit int) (EventStream[types.StartLiveTailResponseStream], error) {
	ctx, cancel := context.WithCancel(ctx)
	es := &eventStream{
		cancel: cancel,
		ch:     make(chan types.StartLiveTailResponseStream),
	}

	lgi := LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForTaskID(taskID)}}
	if err := QueryLogGroup(ctx, lgi, start, end, func(events []LogEvent) error {
		es.ch <- &types.StartLiveTailResponseStreamMemberSessionUpdate{
			Value: types.LiveTailSessionUpdate{SessionResults: events},
		}
		return nil
	}); err != nil {
		es.err = err
	}

	if limit > 0 {
		es.ch = takeLastN(es.ch, limit)
	}

	return es, nil
}

func (a *AwsEcs) TailTaskID(ctx context.Context, taskID string) (LiveTailStream, error) {
	if taskID == "" {
		return nil, errors.New("taskID is empty")
	}
	lgi := LogGroupInput{LogGroupARN: a.LogGroupARN, LogStreamNames: []string{GetCDLogStreamForTaskID(taskID)}}
	for {
		stream, err := TailLogGroup(ctx, lgi)
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
