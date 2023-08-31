package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

func Logs(ctx context.Context, arn pulumi.TaskArn, region aws.Region) error {
	awsecs := pulumi.New(stack, region)
	return awsecs.Tail(ctx, arn)
}
