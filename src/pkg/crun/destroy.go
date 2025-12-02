package crun

import (
	"context"
)

func Destroy(ctx context.Context, region Region) error {
	driver, err := createDriver(region)
	if err != nil {
		return err
	}
	return driver.TearDown(ctx)
}
