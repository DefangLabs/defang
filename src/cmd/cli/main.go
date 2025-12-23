package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/DefangLabs/defang/src/cmd/cli/command"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			track.Evt("Panic", track.P("version", version), track.P("error", r), track.P("callstack", string(skipLines(debug.Stack(), 6))))
			track.FlushAllTracking()
			panic(r)
		}
	}()

	// To avoid creating files as root, we set the UID and GID to the "current" HOME user
	if homeDir, err := os.UserHomeDir(); err == nil {
		setUidGidFromFile(homeDir)
	}

	// Handle Ctrl+C so we can exit gracefully
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)

	slog.SetDefault(logs.NewTermLogger(term.DefaultTerm))
	command.SetupCommands(version)
	err := command.Execute(ctx)
	if ctx.Err() != nil {
		// The context was cancelled by the Interrupt signal handler
		track.Evt("User Interrupted", track.P("version", version))
	}
	stop()
	track.FlushAllTracking()

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
