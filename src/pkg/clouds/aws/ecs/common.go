package ecs

import (
	"strings"

	"github.com/defang-io/defang/src/pkg/aws"
	"github.com/defang-io/defang/src/pkg/types"
)

const (
	ContainerName     = "main"
	DockerRegistry    = "docker.io"
	EcrPublicRegistry = "public.ecr.aws"
	ProjectName       = types.ProjectName
)

type TaskArn = types.TaskID

type AwsEcs struct {
	aws.Aws
	BucketName      string
	ClusterName     string
	LogGroupARN     string
	SecurityGroupID string
	Spot            bool
	SubNetID        string
	TaskDefARN      string
	VpcID           string
}

func PlatformToArchOS(platform string) (string, string) {
	parts := strings.SplitN(platform, "/", 3) // Can be "os/arch/variant" like "linux/arm64/v8"

	if len(parts) == 1 {
		arch := parts[0]
		return normalizedArch(arch), ""
	} else {
		os := parts[0]
		arch := parts[1]
		os = strings.ToUpper(os)
		return normalizedArch(arch), os
	}
}

func normalizedArch(arch string) string {
	// From https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-ecs-taskdefinition-runtimeplatform.html#cfn-ecs-taskdefinition-runtimeplatform-cpuarchitecture
	arch = strings.ToUpper(arch)
	if arch == "AMD64" {
		arch = "X86_64"
	}
	return arch
}

func (a *AwsEcs) SetVpcID(vpcId string) error {
	a.VpcID = vpcId
	return nil
}

func (a *AwsEcs) GetVpcID() string {
	return a.VpcID
}

func (a *AwsEcs) getAccountID() string {
	return aws.GetAccountID(a.TaskDefARN)
}

func (a *AwsEcs) MakeARN(service, resource string) string {
	return strings.Join([]string{
		"arn",
		"aws",
		service,
		string(a.Region),
		a.getAccountID(),
		resource,
	}, ":")
}
