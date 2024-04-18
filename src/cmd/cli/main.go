package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/defang-io/defang/src/cmd/cli/command"
	"github.com/defang-io/defang/src/pkg/term"
)

func main() {
	// Handle Ctrl+C so we can exit gracefully
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		term.Debug("Received interrupt signal; cancelling...")
		command.Track("User Interrupted")
		command.FlushAllTracking()
		cancel()
	}()

	command.SetupCommands()
	command.Execute(ctx)
	command.FlushAllTracking()
}
