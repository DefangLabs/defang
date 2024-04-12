package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/defang-io/defang/src/pkg/cmd"
	"github.com/spf13/pflag"
)

var (
	help     = pflag.BoolP("help", "h", false, "Show this help message")
	color    = pflag.String("color", "auto", `Colorize output. Choices are: always, never, raw, auto`)
	envs     = pflag.StringArrayP("env", "e", nil, "Environment variables to pass to the run command")
	region   = pflag.StringP("region", "r", os.Getenv("AWS_REGION"), "Which cloud region to use, or blank for local Docker")
	memory   = pflag.StringP("memory", "m", "2g", "Memory limit in bytes")
	envFiles = pflag.StringArray("env-file", nil, "Read in a file of environment variables")
	platform = pflag.String("platform", "", "Set platform if host is multi-platform capable")
	vpcid    = pflag.String("vpcid", "", "VPC to use for the task")
	subnetid = pflag.String("subnetid", "", "Subnet to use for the task")
	// driver = pflag.StringP("driver", "d", "auto", "Container runner to use. Choices are: pulumi-ecs, docker")

	version = "development" // overwritten by build script -ldflags "-X main.version=..."
)

func usage() {
	fmt.Printf("Cloud runner (%s)\n\n", version)
	pflag.Usage()
	fmt.Println(`
Commands:
  run <image> [arg...]   Create and run a new task from an image
  logs <task ID>         Fetch the logs of a task
  stop <task ID>         Stop a running task
  info <task ID>         Show information about a task
  destroy                Destroy all resources created by this tool`)
}

func main() {
	pflag.Parse()
	if *help {
		usage()
		return
	}

	region := cmd.Region(*region)

	envMap := make(map[string]string)
	// Apply env vars from files first, so they can be overridden by the command line
	for _, envFile := range *envFiles {
		if _, err := cmd.ParseEnvFile(envFile, envMap); err != nil {
			cmd.Fatal(err)
		}
	}
	// Apply env vars from the command line last, so they take precedence
	for _, env := range *envs {
		if key, value := cmd.ParseEnvLine(env); key != "" {
			envMap[key] = value
		}
	}

	ctx := context.Background()

	requireNoFlags := func() {
		if len(envMap) > 0 || *platform != "" || *vpcid != "" || *subnetid != "" {
			cmd.Fatal("stop does not take --env, --env-file, --platform, --vpcid, or --subnetid")
		}
	}
	requireTaskIDForCmd := func(command string) {
		if pflag.NArg() != 2 {
			cmd.Fatal(command + " requires a single task ID")
		}
	}

	var err error
	switch pflag.Arg(0) {
	default:
		err = errors.New("unknown command: " + pflag.Arg(0))
	case "help", "":
		usage()
	case "run", "r":
		if pflag.NArg() < 2 {
			cmd.Fatal("run requires an image name (and optional arguments)")
		}
		memory := cmd.ParseMemory(*memory)
		err = cmd.Run(ctx, cmd.RunContainerArgs{
			Region:   region,
			Image:    pflag.Arg(1),
			Memory:   memory,
			Args:     pflag.Args()[2:],
			Env:      envMap,
			Platform: *platform,
			VpcID:    *vpcid,
			SubnetID: *subnetid,
		})
	case "stop", "s":
		requireNoFlags()
		requireTaskIDForCmd("stop")
		taskID := pflag.Arg(1)
		err = cmd.Stop(ctx, region, &taskID)
	case "logs", "tail", "l":
		requireNoFlags()
		requireTaskIDForCmd("logs")
		taskID := pflag.Arg(1)
		err = cmd.Logs(ctx, region, &taskID)
	case "destroy", "teardown", "d":
		requireNoFlags()
		if pflag.NArg() != 1 {
			cmd.Fatal("destroy does not take any arguments")
		}
		err = cmd.Destroy(ctx, region)
	case "info", "i":
		requireNoFlags()
		requireTaskIDForCmd("info")
		taskID := pflag.Arg(1)
		err = cmd.Info(ctx, region, &taskID)
	}

	if err != nil {
		cmd.Fatal(err)
	}
}
