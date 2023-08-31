package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

func Run(ctx context.Context, image string, color Color, region aws.Region, args []string, env map[string]string) error {
	awsecs := pulumi.New(stack, region)
	if err := awsecs.SetUp(ctx, image, pulumi.Color(color)); err != nil {
		return err
	}

	arn, err := awsecs.Run(ctx, env, args...)
	if err != nil {
		return err
	}

	println("Task ARN:", *arn)
	return awsecs.Tail(ctx, arn)
}
