package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/defang-io/defang/src/pkg/types"
)

func Run(ctx context.Context, region Region, image string, memory uint64, color Color, args []string, env map[string]string, platform, vpcId string) error {
	driver := createDriver(color, region)

	if err := driver.SetVpcID(vpcId); err != nil { // VPC affects the cloudformation template
		return err
	}

	containers := []types.Container{
		{
			Image:    image,
			Memory:   memory,
			Platform: platform,
		},
	}
	if err := driver.SetUp(ctx, containers); err != nil {
		return err
	}

	id, err := driver.Run(ctx, env, args...)
	if err != nil {
		return err
	}

	fmt.Println("Task ID:", *id)

	// Try 10 times to get the public IP address
	for i := 0; i < 10; i++ {
		info, err := driver.GetInfo(ctx, id)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if info != nil {
			fmt.Println("IP:", info.IP)
		}
		break
	}

	// FIXME: stop task on context cancelation/ctrl-c?
	return driver.Tail(ctx, id)
}
