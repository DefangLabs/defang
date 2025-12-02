package crun

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/cfn"
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

var cleanup = make(chan func())

func init() {
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		var clanupFns []func()

		for {
			select {
			case f := <-cleanup:
				clanupFns = append(clanupFns, f)
			case s := <-c:
				fmt.Printf("Caught signal %v, cleaning up...\n", s)
				for _, f := range clanupFns {
					f()
				}
				os.Exit(0)
			}
		}
	}()
}

func Run(ctx context.Context, args RunContainerArgs) error {
	driver, err := createDriver(args.Region, cfn.OptionVPCAndSubnetID(ctx, args.VpcID, args.SubnetID))
	if err != nil { // VPC affects the cloudformation template
		return err
	}

	containers := []clouds.Container{
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

	cleanup <- func() {
		fmt.Printf("Stopping task %s...\n", *id)
		driver.Stop(ctx, id)
	}
	fmt.Println("Task ID:", *id)

	// Try 10 times to get the public IP address
	for range 10 {
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
