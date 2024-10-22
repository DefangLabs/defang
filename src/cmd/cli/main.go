package main

import (
	"bytes"
	"context"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/DefangLabs/defang/src/cmd/cli/command"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			command.Track("Panic", command.P{"version", version}, command.P{"error", r}, command.P{"stack", string(skipLines(debug.Stack(), 6))})
			command.FlushAllTracking()
			panic(r)
		}
	}()

	// Handle Ctrl+C so we can exit gracefully
	ctx, cancel := context.WithCancel(context.Background())
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt)

	go func() {
		<-sigs
		signal.Stop(sigs)
		term.Debug("Received interrupt signal; canceling...")
		command.Track("User Interrupted", command.P{"version", version})
		cancel()
	}()

	command.SetupCommands(version)
	err := command.Execute(ctx)
	command.FlushAllTracking() // TODO: track errors/panics

	if err != nil {
		// If the error is a command.ExitCode, use its value as the exit code
		ec, ok := err.(command.ExitCode)
		if !ok {
			ec = 1 // should not happen since we always return ExitCode
		}
		os.Exit(int(ec))
	}
}

// skipLines returns buf with the first n lines removed.
func skipLines(buf []byte, n int) []byte {
	lines := bytes.SplitN(buf, []byte{'\n'}, n)
	return lines[len(lines)-1]
}
