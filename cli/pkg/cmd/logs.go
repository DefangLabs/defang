package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
)

func Logs(ctx context.Context, arn pulumi.TaskArn, region Region) error {
	driver := createDriver("auto", region)
	return driver.Tail(ctx, arn)
}
