package ecs

import (
	"strings"

	"github.com/defang-io/defang/src/pkg/aws"
	"github.com/defang-io/defang/src/pkg/types"
)

const (
	ContainerName = "main"
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
