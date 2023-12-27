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
	"github.com/defang-io/defang/src/pkg/aws/region"
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

	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	// Use CloudWatch API to tail the logs
	cw := cloudwatchlogs.NewFromConfig(cfg)

	spinMe := 0
	var nextToken *string
	for {
		nextToken, err = a.printLogEvents(ctx, cw, logStreamName, nextToken)
		if err != nil {
			return err
		}

		// Use DescribeTasks API to check if the task is still running
		ti, _ := ecs.NewFromConfig(cfg).DescribeTasks(ctx, &ecs.DescribeTasksInput{
			Cluster: aws.String(a.ClusterARN), // arn:aws:ecs:us-west-2:532501343364:cluster/ecs-dev-cluster
			Tasks:   []string{taskId},
		})
		if ti != nil && len(ti.Tasks) > 0 {
			task := ti.Tasks[0]
			switch task.StopCode {
			default:
				// Before we exit, grab any remaining logs
				a.printLogEvents(ctx, cw, logStreamName, nextToken)
				return taskFailure{string(task.StopCode), *task.StoppedReason}
			case ecsTypes.TaskStopCodeEssentialContainerExited:
				// Before we exit, grab any remaining logs
				a.printLogEvents(ctx, cw, logStreamName, nextToken)
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

func (a AwsEcs) printLogEvents(ctx context.Context, cw *cloudwatchlogs.Client, logStreamName string, nextToken *string) (*string, error) {
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
				return nil, err
			}
			break // continue outer loop, waiting for the log stream to be created
		}
		for _, event := range events.Events {
			fmt.Println(*event.Message)
		}
		if nextToken != nil && *nextToken == *events.NextForwardToken {
			break // no new logs
		}
		nextToken = events.NextForwardToken
	}
	return nextToken, nil
}
