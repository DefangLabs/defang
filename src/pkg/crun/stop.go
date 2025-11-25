package crun

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/clouds"
)

func Stop(ctx context.Context, region Region, id clouds.TaskID) error {
	driver, err := createDriver(region)
	if err != nil {
		return err
	}
	return driver.Stop(ctx, id)
}
