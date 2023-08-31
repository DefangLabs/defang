package cmd

import (
	"context"
)

func Destroy(ctx context.Context, color Color, region Region) error {
	driver := createDriver(color, region)
	return driver.Destroy(ctx)
}
