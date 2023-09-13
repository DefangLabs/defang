package cmd

import (
	"context"

	"github.com/defang-io/defang/cli/pkg/types"
)

func Logs(ctx context.Context, region Region, arn types.TaskID) error {
	driver := createDriver(ColorAuto, region)
	return driver.Tail(ctx, arn)
}
