package cmd

import (
	"context"

	"github.com/defang-io/defang/src/pkg/types"
)

func Stop(ctx context.Context, region Region, id types.TaskID) error {
	driver := createDriver(ColorAuto, region)
	return driver.Stop(ctx, id)
}
