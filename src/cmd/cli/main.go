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
		command.FlushAllTracking() // needed? Execute should exit once cancelled
		cancel()
	}()

	restore := term.EnableANSI() // FIXME: only enable ANSI if we want colorized output
	defer restore()

	command.SetupCommands()
	err := command.Execute(ctx)
	command.FlushAllTracking() // TODO: track errors/panics

	if err != nil {
		// If the error is a command.ErrorCode, use its value as the exit code
		ec, ok := err.(command.ExitCode)
		if !ok {
			ec = 1 // should not happen since we always return ErrorCode
		}
		os.Exit(int(ec))
	}
}
