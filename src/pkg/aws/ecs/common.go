package ecs

import (
	"context"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/defang-io/defang/src/pkg/aws/region"
	"github.com/defang-io/defang/src/pkg/types"
)

const (
	ContainerName     = "main"
	EcrPublicRegistry = "public.ecr.aws"
	ProjectName       = types.ProjectName
)

type TaskArn = types.TaskID

type AwsEcs struct {
	ClusterARN      string
	LogGroupName    string
	Region          region.Region
	SecurityGroupID string
	Spot            bool
	SubNetID        string
	TaskDefARN      string
	VCpu            float64
	VpcID           string
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
	// From https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ecs-taskdefinition-runtimeplatform.html#cfn-ecs-taskdefinition-runtimeplatform-cpuarchitecture
	arch = strings.ToUpper(arch)
	if arch == "AMD64" {
		arch = "X86_64"
	}
	return &arch
}

func (a *AwsEcs) SetVpcID(vpcId string) error {
	a.VpcID = vpcId
	return nil
}
