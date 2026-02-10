package ecs

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/smithy-go/ptr"
)

func (a AwsEcs) Stop(ctx context.Context, id clouds.TaskID) error {
	cfg, err := a.LoadConfigForCD(ctx)
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
