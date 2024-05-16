package ecs

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/smithy-go/ptr"
)

func (a AwsEcs) Stop(ctx context.Context, id types.TaskID) error {
	cfg, err := a.LoadConfig(ctx)
	if err != nil {
		return err
	}

	_, err = ecs.NewFromConfig(cfg).StopTask(ctx, &ecs.StopTaskInput{
		Cluster: ptr.String(a.ClusterName),
		Task:    id,
		// Reason: ptr.String("defang stop"),
	})
	return err
}
