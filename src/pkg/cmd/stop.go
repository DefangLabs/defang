package cmd

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
)

func Stop(ctx context.Context, region Region, id types.TaskID) error {
	driver, err := createDriver(region)
	if err != nil {
		return err
	}
	return driver.Stop(ctx, id)
}
