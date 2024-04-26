package main // this must be "main" or -ldflags will fail to set the version

import "github.com/defang-io/defang/src/cmd/cli/command"

var version = "development" // overwritten by build script -ldflags "-X main.version=..." and GoReleaser

func init() {
	command.RootCmd.Version = version
}
