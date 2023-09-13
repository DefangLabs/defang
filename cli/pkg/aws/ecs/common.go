package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/defang-io/defang/cli/pkg/aws/region"
	"github.com/defang-io/defang/cli/pkg/types"
)

const (
	ContainerName     = "main"
	EcrPublicRegistry = "public.ecr.aws"
	ProjectName       = types.ProjectName
)

type TaskArn = types.TaskID

type AwsEcs struct {
	Spot         bool
	VCpu         float64
	ClusterARN   string
	LogGroupName string
	Region       region.Region
	TaskDefARN   string
	SubnetID     string
}

func (a AwsEcs) LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)))
}
