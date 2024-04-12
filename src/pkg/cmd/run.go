package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/defang-io/defang/src/pkg/clouds/aws/ecs/cfn"
	"github.com/defang-io/defang/src/pkg/types"
)

type RunContainerArgs struct {
	Region   Region
	Image    string
	Memory   uint64
	Args     []string
	Env      map[string]string
	Platform string
	VpcID    string
	SubnetID string
}

func Run(ctx context.Context, args RunContainerArgs) error {
	driver, err := createDriver(args.Region, cfn.OptionVPCAndSubnetID(ctx, args.VpcID, args.SubnetID))
	if err != nil { // VPC affects the cloudformation template
		return err
	}

	containers := []types.Container{
		{
			Image:    args.Image,
			Memory:   args.Memory,
			Platform: args.Platform,
		},
	}
	if err := driver.SetUp(ctx, containers); err != nil {
		return err
	}

	id, err := driver.Run(ctx, args.Env, args.Args...)
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
