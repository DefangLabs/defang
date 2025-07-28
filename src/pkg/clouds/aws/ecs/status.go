package ecs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// GetTaskStatus returns nil if the task is still running, io.EOF if the task is stopped successfully, or an error if the task failed.
func GetTaskStatus(ctx context.Context, taskArn TaskArn) error {
	region := aws.RegionFromArn(*taskArn)
	cluster, taskID := SplitClusterTask(taskArn)
	return getTaskStatus(ctx, region, cluster, taskID)
}

func isTaskTerminalStatus(status string) bool {
	// From https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task-lifecycle-explanation.html
	switch status {
	case "DELETED", "STOPPED", "DEPROVISIONING":
		return true
	default:
		return false // we might still get logs
	}
}

// getTaskStatus returns nil if the task is still running, io.EOF if the task is stopped successfully, or an error if the task failed.
func getTaskStatus(ctx context.Context, region aws.Region, cluster, taskId string) error {
	cfg, err := aws.LoadDefaultConfig(ctx, region)
	if err != nil {
		return err
	}
	ecsClient := ecs.NewFromConfig(cfg)

	// Use DescribeTasks API to check if the task is still running (same as ecs.NewTasksStoppedWaiter)
	ti, _ := ecsClient.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: &cluster,
		Tasks:   []string{taskId},
	})
	if ti == nil || len(ti.Tasks) == 0 {
		return nil // task doesn't exist yet; TODO: check the actual error from DescribeTasks
	}
	task := ti.Tasks[0]
	if task.LastStatus == nil || !isTaskTerminalStatus(*task.LastStatus) {
		return nil // still running
	}

	switch task.StopCode {
	default:
		return TaskFailure{task.StopCode, *task.StoppedReason}
	case ecsTypes.TaskStopCodeEssentialContainerExited:
		for _, c := range task.Containers {
			if c.ExitCode != nil && *c.ExitCode != 0 {
				reason := fmt.Sprintf("%s with code %d", *task.StoppedReason, *c.ExitCode)
				return TaskFailure{task.StopCode, reason}
			}
		}
		fallthrough
	case "": // TODO: shouldn't happen
		return io.EOF // Success
	}
}

func SplitClusterTask(taskArn TaskArn) (string, string) {
	if !strings.HasPrefix(*taskArn, "arn:aws:ecs:") {
		panic("invalid ECS ARN")
	}
	parts := strings.Split(*taskArn, "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[0], ":task") {
		panic("invalid task ARN")
	}
	return parts[1], parts[2]
}

// WaitForTask polls the ECS task status. It returns io.EOF if the task is stopped successfully, or an error if the task failed.
func WaitForTask(ctx context.Context, taskArn TaskArn, poll time.Duration) error {
	if taskArn == nil {
		panic("taskArn is nil")
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Handle cancellation
			return ctx.Err()
		case <-ticker.C:
			if err := GetTaskStatus(ctx, taskArn); err != nil {
				return err
			}
		}
	}
}
