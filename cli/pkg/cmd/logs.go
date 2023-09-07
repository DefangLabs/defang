package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/aws/ecs/pulumi"
)

func Logs(ctx context.Context, region Region, arn pulumi.TaskArn) error {
	driver := createDriver(ColorAuto, region)
	return driver.Tail(ctx, arn)
}
