package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/crun"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/pflag"
)

var (
	_      = pflag.BoolP("help", "h", false, "Show this help message")
	region = pflag.StringP("region", "r", os.Getenv("AWS_REGION"), "Which cloud region to use, or blank for local Docker")

	runFlags = pflag.NewFlagSet(os.Args[0]+" run", pflag.ExitOnError)
	envs     = runFlags.StringArrayP("env", "e", nil, "Environment variables to pass to the run command")
	memory   = runFlags.StringP("memory", "m", "2g", "Memory limit in bytes")
	envFiles = runFlags.StringArray("env-file", nil, "Read in a file of environment variables")
	platform = runFlags.String("platform", "", "Set platform if host is multi-platform capable")
	vpcid    = runFlags.String("vpcid", "", "VPC to use for the task")
	subnetid = runFlags.String("subnetid", "", "Subnet to use for the task")
	// driver = pflag.StringP("driver", "d", "auto", "Container runner to use. Choices are: pulumi-ecs, docker")

	version = "development" // overwritten by build script -ldflags "-X main.version=..."
)

func init() {
	runFlags.StringVarP(region, "region", "r", os.Getenv("AWS_REGION"), "Which cloud region to use, or blank for local Docker")
}

func usage() {
	fmt.Printf("Cloud runner (%s)\n\n", version)
	fmt.Printf("Usage: \n  %s [command] [options]\n\nGlobal Flags:\n", os.Args[0])
	pflag.PrintDefaults()
	fmt.Println(`
Commands:
  run <image> [arg...]   Create and run a new task from an image
  logs <task ID>         Fetch the logs of a task
  stop <task ID>         Stop a running task
  info <task ID>         Show information about a task
  destroy                Destroy all resources created by this tool`)
	fmt.Printf("\n\nUsage of run subcommand: \n  %s run [options]\n\nFlags:\n", os.Args[0])
	runFlags.PrintDefaults()
}

func main() {
	pflag.Usage = usage
	if len(os.Args) < 2 {
		usage()
		return
	}

	command := os.Args[1]

	if command == "run" || command == "r" {
		runFlags.Parse(os.Args[2:])
	} else {
		pflag.Parse()
	}

	region := crun.Region(*region)
	ctx := context.Background()

	requireTaskID := func() string {
		if pflag.NArg() != 2 {
			term.Fatal(command + " requires a single task ID argument")
		}
		return pflag.Arg(1)
	}

	var err error
	switch command {
	default:
		err = errors.New("unknown command: " + command)
	case "help", "":
		usage()
	case "run", "r":
		if runFlags.NArg() < 1 {
			term.Fatal("run requires an image name (and optional arguments)")
		}

		envMap := make(map[string]string)
		// Apply env vars from files first, so they can be overridden by the command line
		for _, envFile := range *envFiles {
			if _, err := crun.ParseEnvFile(envFile, envMap); err != nil {
				term.Fatal(err)
			}
		}
		// Apply env vars from the command line last, so they take precedence
		for _, env := range *envs {
			if key, value := crun.ParseEnvLine(env); key != "" {
				envMap[key] = value
			}
		}

		memory := crun.ParseMemory(*memory)
		err = crun.Run(ctx, crun.RunContainerArgs{
			Region:   region,
			Image:    runFlags.Arg(0),
			Memory:   memory,
			Args:     runFlags.Args()[1:],
			Env:      envMap,
			Platform: *platform,
			VpcID:    *vpcid,
			SubnetID: *subnetid,
		})
	case "stop", "s":
		taskID := requireTaskID()
		err = crun.Stop(ctx, region, &taskID)
	case "logs", "tail", "l":
		taskID := requireTaskID()
		err = crun.Logs(ctx, region, &taskID)
	case "destroy", "teardown", "d":
		if pflag.NArg() != 1 {
			term.Fatal("destroy does not take any arguments")
		}
		err = crun.Destroy(ctx, region)
	case "info", "i":
		taskID := requireTaskID()
		err = crun.PrintInfo(ctx, region, &taskID)
	}

	if err != nil {
		term.Fatal(err)
	}
}
