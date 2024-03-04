package lambda

import "github.com/defang-io/defang/src/pkg/aws"

// type TaskArn = types.TaskID

type AwsLambda struct {
	aws.Aws
	BucketName      string
	LogGroupARN     string
	SecurityGroupID string
	SubNetID        string
	VpcID           string
	FunctionName    string
}
