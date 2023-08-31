package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
	"github.com/pulumi/pulumi-aws/sdk/v5/go/aws"
)

func Destroy(ctx context.Context, color Color, region aws.Region) error {
	awsecs := pulumi.New(stack, region)
	return awsecs.Destroy(ctx, pulumi.Color(color))
}
