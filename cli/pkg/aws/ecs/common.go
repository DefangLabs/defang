package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/defang-io/defang/cli/pkg/types"
	pulumiAws "github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

const (
	ContainerName     = "main"
	EcrPublicRegistry = "public.ecr.aws"
	StreamPrefix      = "crun"
)

type Region = pulumiAws.Region // TODO: don't use Pulumi's Region type

type TaskArn = types.TaskID

type AwsEcs struct {
	Spot         bool
	VCpu         float64
	ClusterArn   string
	LogGroupName string
	Region       Region
	TaskDefArn   string
}

func (a AwsEcs) LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)))
}
