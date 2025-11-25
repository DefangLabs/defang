package crun

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/clouds"
)

func PrintInfo(ctx context.Context, region Region, id clouds.TaskID) error {
	driver, err := createDriver(region)
	if err != nil {
		return err
	}
	info, err := driver.GetInfo(ctx, id)
	if err != nil {
		return err
	}
	fmt.Println("IP:", info.IP)
	return nil
}
