package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"sync"

	"github.com/DefangLabs/defang/src/cmd/cli/command"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
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

	// Set up always-on debug log file; all log levels are written here.
	// defer handles the normal-exit path; the explicit call handles os.Exit.
	flushDebugLog := setupDebugLog()
	defer flushDebugLog()

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
		flushDebugLog() // os.Exit skips defers, so flush explicitly
		os.Exit(int(ec))
	}
}

// skipLines returns buf with the first n lines removed.
func skipLines(buf []byte, n int) []byte {
	lines := bytes.SplitN(buf, []byte{'\n'}, n)
	return lines[len(lines)-1]
}

// setupDebugLog creates a temp debug log file, wires it into DefaultTerm, and
// returns a flush function that closes and atomically renames the file to
// debug.log. The flush function is idempotent — safe to call multiple times.
func setupDebugLog() func() {
	stateDir := client.StateDir
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return func() {}
	}

	finalPath := filepath.Join(stateDir, "debug.log")
	f, err := os.CreateTemp(stateDir, "debug-*.log")
	if err != nil {
		return func() {}
	}
	tmpPath := f.Name()

	term.SetDebugLog(f)

	var once sync.Once
	return func() {
		once.Do(func() {
			term.SetDebugLog(nil)
			f.Close()
			os.Rename(tmpPath, finalPath) //nolint:errcheck
			if term.DoDebug() {
				term.Infof("Debug log written to: %s", finalPath)
			}
		})
	}
}
