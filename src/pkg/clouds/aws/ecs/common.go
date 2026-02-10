package ecs

import (
	"context"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	awsx "github.com/aws/aws-sdk-go-v2/aws"
)

const (
	CdContainerName   = "main"
	DockerRegistry    = "docker.io"
	EcrPublicRegistry = "public.ecr.aws"
	CrunProjectName   = "defang"
)

type TaskArn = clouds.TaskID

type AwsEcs struct {
	aws.Aws
	CDRegion               aws.Region
	BucketName             string
	CIRoleARN              string
	ClusterName            string
	DefaultSecurityGroupID string
	LogGroupARN            string
	RetainBucket           bool
	SecurityGroupID        string
	Spot                   bool
	SubNetID               string
	TaskDefARN             string
	VpcID                  string
}

func (a *AwsEcs) LoadConfigForCD(ctx context.Context) (awsx.Config, error) {
	cfg, err := aws.LoadDefaultConfig(ctx, a.CDRegion)
	// If we don't have an region override for CD, use the current AWS region
	if a.CDRegion == "" {
		a.CDRegion = aws.Region(cfg.Region)
	}
	return cfg, err
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

func (a *AwsEcs) GetVpcID() string {
	return a.VpcID
}

func (a *AwsEcs) MakeCdARN(service, resource string) string {
	return strings.Join([]string{
		"arn",
		"aws",
		service,
		string(a.CDRegion),
		a.AccountID,
		resource,
	}, ":")
}
