package cmd

import (
	"context"
)

func Destroy(ctx context.Context, region Region, color Color) error {
	driver := createDriver(color, region)
	return driver.TearDown(ctx)
}
