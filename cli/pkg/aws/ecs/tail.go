package ecs

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/defang-io/defang/cli/pkg/aws/region"
)

const spinner = `-\|/`

func (a *AwsEcs) Tail(ctx context.Context, taskArn TaskArn) error {
	// a.Refresh(ctx)

	parts := strings.Split(*taskArn, ":")

	if len(parts) == 6 {
		a.Region = region.Region(parts[3])
	}

	taskId := path.Base(*taskArn)
	logStreamName := path.Join(ProjectName, ContainerName, taskId)

	// Use CloudWatch API to tail the logs
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	cw := cloudwatchlogs.NewFromConfig(cfg)

	spinMe := 0
	var nextToken *string
	for {
		events, err := cw.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  aws.String(a.LogGroupName),
			LogStreamName: aws.String(logStreamName),
			NextToken:     nextToken,
			StartFromHead: aws.Bool(true),
		})
		if err != nil {
			var resourceNotFound *types.ResourceNotFoundException
			if !errors.As(err, &resourceNotFound) {
				return err
			}
			// Continue, waiting for the log stream to be created
		} else {
			for _, event := range events.Events {
				println(*event.Message)
			}
			if nextToken == nil || *nextToken != *events.NextForwardToken {
				nextToken = events.NextForwardToken
				continue
			}
		}

		// Use DescribeTasks API to check if the task is still running
		tasks, _ := ecs.NewFromConfig(cfg).DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(a.ClusterARN), // arn:aws:ecs:us-west-2:532501343364:cluster/ecs-dev-cluster
			Tasks:   []string{taskId},
		})
		if tasks != nil && len(tasks.Tasks) > 0 {
			task := tasks.Tasks[0]
			switch task.StopCode {
			default:
				return taskFailure{string(task.StopCode), *task.StoppedReason}
			case ecsTypes.TaskStopCodeEssentialContainerExited:
				if *task.Containers[0].ExitCode == 0 {
					return nil // Success
				}
				reason := fmt.Sprintf("%s with code %d", *task.StoppedReason, *task.Containers[0].ExitCode)
				return taskFailure{string(task.StopCode), reason}
			case "": // Task is still running
			}
		}

		fmt.Printf("%c\r", spinner[spinMe%len(spinner)])
		spinMe++
		time.Sleep(1 * time.Second)
	}
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
