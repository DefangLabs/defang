package main

import (
	"context"
	"fmt"
	"os"

	"github.com/defang-io/defang/cli/pkg/cmd"
	"github.com/spf13/pflag"
)

var (
	color    = pflag.String("color", "auto", `Colorize output. Choices are: always, never, raw, auto`)
	envs     = pflag.StringArrayP("env", "e", nil, "Environment variables to pass to the run command")
	region   = pflag.StringP("region", "r", os.Getenv("AWS_REGION"), "Which cloud region to use, or blank for local Docker")
	memory   = pflag.StringP("memory", "m", "2g", "Memory limit in bytes")
	envFiles = pflag.StringArray("env-file", nil, "Read in a file of environment variables")
	// driver = pflag.StringP("driver", "d", "auto", "Container runner to use. Choices are: pulumi-ecs, docker")

	version = "development" // overwritten by build script -ldflags "-X main.version=..."
)

func usage() {
	fmt.Printf("Cloud runner (%s)\n\n", version)
	pflag.Usage()
	fmt.Println(`
Commands:
  run <image> [args]
  logs <task ID>
  destroy`)
}

func main() {
	pflag.Parse()
	color := cmd.ParseColor(*color)
	region := cmd.Region(*region)
	memory := cmd.ParseMemory(*memory)

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

	var err error
	switch pflag.Arg(0) {
	default:
		usage()
	case "run":
		if pflag.NArg() < 2 {
			cmd.Fatal("run requires an image name (and optional arguments)")
		}
		err = cmd.Run(ctx, region, pflag.Arg(1), memory, color, pflag.Args()[2:], envMap)
	case "logs", "tail":
		if pflag.NArg() != 2 {
			cmd.Fatal("logs requires a single task ID")
		}
		taskArn := pflag.Arg(1)
		err = cmd.Logs(ctx, region, &taskArn)
	case "destroy", "teardown":
		if pflag.NArg() != 1 {
			cmd.Fatal("destroy does not take any arguments")
		}
		err = cmd.Destroy(ctx, region, color)
	}

	if err != nil {
		cmd.Fatal(err)
	}
}
