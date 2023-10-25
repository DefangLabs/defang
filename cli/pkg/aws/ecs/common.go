package ecs

import (
	"context"
	"strings"

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
	Spot            bool
	VCpu            float64
	ClusterARN      string
	LogGroupName    string
	Region          region.Region
	TaskDefARN      string
	SubnetID        string
	SecurityGroupID string
}

func (a AwsEcs) LoadConfig(ctx context.Context) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx, config.WithRegion(string(a.Region)))
}

func PlatformToArch(platform string) *string {
	if platform == "" {
		return nil
	}
	parts := strings.SplitN(platform, "/", 3)
	arch := parts[0]
	if len(parts) >= 2 {
		arch = parts[1]
	}
	arch = strings.ToLower(arch)
	if arch == "AMD64" {
		arch = "X86_64"
	}
	return &arch
}
