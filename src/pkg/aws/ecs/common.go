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
	VCpu            float64
	VpcID           string
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
