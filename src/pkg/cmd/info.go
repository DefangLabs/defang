package cmd

import (
	"context"
	"fmt"

	"github.com/defang-io/defang/src/pkg/types"
)

func Info(ctx context.Context, region Region, id types.TaskID) error {
	driver := createDriver(ColorAuto, region)
	info, err := driver.GetInfo(ctx, id)
	if err != nil {
		return err
	}
	fmt.Println(info)
	return nil
}
