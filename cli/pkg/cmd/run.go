package cmd

import (
	"context"
	"fmt"
	"time"
)

func Run(ctx context.Context, region Region, image string, memory uint64, color Color, args []string, env map[string]string, platform string) error {
	driver := createDriver(color, region)
	if err := driver.SetUp(ctx, image, memory, platform); err != nil {
		return err
	}

	id, err := driver.Run(ctx, env, args...)
	if err != nil {
		return err
	}

	fmt.Println("Task ID:", *id)

	// Try 10 times to get the public IP address
	for i := 0; i < 10; i++ {
		info, err := driver.Info(ctx, id)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if info != "" {
			fmt.Println(info)
		}
		break
	}

	// FIXME: stop task on context cancelation/ctrl-c?
	return driver.Tail(ctx, id)
}
