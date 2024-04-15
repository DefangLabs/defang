package cmd

import (
	"context"

	"github.com/defang-io/defang/src/pkg/types"
)

func Logs(ctx context.Context, region Region, id types.TaskID) error {
	driver, err := createDriver(region)
	if err != nil {
		return err
	}
	return driver.Tail(ctx, id)
}
