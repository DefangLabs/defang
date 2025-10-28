package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/spinner"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/muesli/termenv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ansiCyan      = "\033[36m"
	ansiReset     = "\033[0m"
	replaceString = ansiCyan + "$0" + ansiReset
	RFC3339Milli  = "2006-01-02T15:04:05.000Z07:00" // like RFC3339Nano but with 3 digits of precision
)

var (
	colorKeyRegex = regexp.MustCompile(`"(?:\\["\\/bfnrt]|[^\x00-\x1f"\\]|\\u[0-9a-fA-F]{4})*"\s*:|[^\x00-\x20"=&?]+=`) // handles JSON, logfmt, and query params
)

// Deprecated: use Subscribe instead #851
type TailDetectStopEventFunc func(eventLog *defangv1.LogEntry) error

type TailOptions struct {
	EndEventDetectFunc TailDetectStopEventFunc // Deprecated: use Subscribe and GetDeploymentStatus instead #851
	Deployment         types.ETag
	Filter             string
	LogType            logs.LogType
	Raw                bool
	Services           []string
	Since              time.Time
	Until              time.Time
	Verbose            bool
	Follow             bool
}

func (to TailOptions) String() string {
	cmd := " --since=" + to.Since.UTC().Format(time.RFC3339Nano)
	if to.Until.IsZero() {
		// No --until implies --follow
		cmd = "tail" + cmd
	} else {
		cmd = "logs" + cmd + " --until=" + to.Until.UTC().Format(time.RFC3339Nano)
	}
	if to.Follow {
		cmd += " --follow"
	}
	if to.Deployment != "" {
		cmd += " --deployment=" + to.Deployment
	}
	if to.Raw {
		cmd += " --raw"
	}
	// --verbose is the default for "tail" so we test for false
	if !to.Verbose {
		cmd += " --verbose=0"
	}
	if to.LogType != logs.LogTypeUnspecified {
		cmd += " --type=" + to.LogType.String()
	}
	if to.Filter != "" {
		cmd += fmt.Sprintf(" --filter=%q", to.Filter)
	}
	if len(to.Services) > 0 {
		cmd += " " + strings.Join(to.Services, " ")
	}
	return cmd
}

var P = track.P

// EnableUTCMode sets the local time zone to UTC.
func EnableUTCMode() {
	time.Local = time.UTC
}

// ParseTimeOrDuration parses a time string or duration string (e.g. 1h30m) and returns a time.Time.
// At a minimum, this function supports RFC3339Nano, Go durations, and our own TimestampFormat (local).
func ParseTimeOrDuration(str string, now time.Time) (time.Time, error) {
	if str == "" {
		return time.Time{}, nil
	}
	if strings.ContainsAny(str, "TZ") {
		return time.Parse(time.RFC3339Nano, str)
	}
	if strings.Contains(str, ":") {
		local, err := time.ParseInLocation("15:04:05.999999", str, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		// Replace the year, month, and day of t with today's date
		now := now.Local()
		sincet := time.Date(now.Year(), now.Month(), now.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), local.Location())
		if sincet.After(now) {
			sincet = sincet.AddDate(0, 0, -1) // yesterday; subtract 1 day
		}
		return sincet, nil
	}
	dur, err := time.ParseDuration(str)
	if err != nil {
		return time.Time{}, err
	}
	return now.Add(-dur), nil // - because we want to go back in time
}

type CancelError struct {
	TailOptions
	ProjectName string
	error
}

func (cerr CancelError) Error() string {
	cmd := cerr.String()
	if cerr.ProjectName != "" {
		cmd += " --project-name=" + cerr.ProjectName
	}
	return cmd
}

func (cerr CancelError) Unwrap() error {
	return cerr.error
}

func Tail(ctx context.Context, provider client.Provider, projectName string, options TailOptions) error {
	if options.LogType == logs.LogTypeUnspecified {
		options.LogType = logs.LogTypeAll
	}

	term.Debugf("Tailing %s logs in project %q", options.LogType, projectName)

	if len(options.Services) > 0 {
		for _, service := range options.Services {
			// Show a warning if the service doesn't exist (yet); TODO: could do fuzzy matching and suggest alternatives
			if _, err := provider.GetService(ctx, &defangv1.GetRequest{Project: projectName, Name: service}); err != nil {
				switch connect.CodeOf(err) {
				case connect.CodeNotFound:
					term.Warnf("Service does not exist (yet): %q", service)
				case connect.CodeUnknown:
					// Ignore unknown (nil) errors
				default:
					term.Warn(err) // TODO: use cliClient.PrettyError(…)
				}
			}
		}
	}

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	return streamLogs(ctx, provider, projectName, options, logEntryPrintHandler)
}

func isTransientError(err error) bool {
	// Networking errors are considered transient errors
	var netOpErr *net.OpError
	if errors.As(err, &netOpErr) && netOpErr.Temporary() {
		return true
	}

	// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
	code := connect.CodeOf(err)
	// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
	if code == connect.CodeUnavailable {
		return true
	}
	if code == connect.CodeInternal && !connect.IsWireError(err) {
		return true
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// GCP grpc transient errors
	if st, ok := status.FromError(err); ok {
		transientCodes := []codes.Code{codes.Unavailable, codes.Internal}
		if slices.Contains(transientCodes, st.Code()) {
			return true
		}
	}

	return false
}

type LogEntryHandler func(*defangv1.LogEntry, *TailOptions) error

const DefaultTailLimit = 100

func streamLogs(ctx context.Context, provider client.Provider, projectName string, options TailOptions, handler LogEntryHandler) error {
	var sinceTs, untilTs *timestamppb.Timestamp
	if pkg.IsValidTime(options.Since) {
		sinceTs = timestamppb.New(options.Since)
	} else {
		options.Since = time.Now() // this is used to continue from the last timestamp
	}
	if pkg.IsValidTime(options.Until) {
		until := options.Until.Add(time.Millisecond) // add a millisecond to make it inclusive
		untilTs = timestamppb.New(until)
		// If the user specifies a deadline in the future, we should respect it
		if until.After(time.Now()) {
			var cancel context.CancelFunc
			ctx, cancel = context.WithDeadline(ctx, until)
			defer cancel()
		}
	}

	limit := int32(0) // 0 means no limit
	if !options.Follow {
		limit = DefaultTailLimit
	}

	tailRequest := &defangv1.TailRequest{
		Etag:     options.Deployment,
		LogType:  uint32(options.LogType),
		Pattern:  options.Filter,
		Project:  projectName,
		Services: options.Services,
		Since:    sinceTs, // this is also used to continue from the last timestamp
		Until:    untilTs,
		Follow:   options.Follow,
		Limit:    limit,
	}

	term.Debug("Tail request:", tailRequest)

	serverStream, err := provider.QueryLogs(ctx, tailRequest)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // to ensure we close the stream and clean-up this context

	go func() {
		<-ctx.Done()
		serverStream.Close() // this works because it takes a pointer receiver
	}()

	spin := spinner.New()
	doSpinner := !options.Raw && term.StdoutCanColor() && term.IsTerminal()

	if term.IsTerminal() && !options.Raw {
		if doSpinner {
			term.HideCursor()
			defer term.ShowCursor()

			cancelSpinner := spin.Start(ctx)
			defer cancelSpinner()
		}

		// HACK: On Windows, closing stdout will cause debugger to stop working
		if !options.Verbose && runtime.GOOS != "windows" {
			// Allow the user to toggle verbose mode with the V key
			if oldState, err := term.MakeUnbuf(int(os.Stdin.Fd())); err == nil {
				defer term.Restore(int(os.Stdin.Fd()), oldState)

				term.Info("Showing only build logs and runtime errors. Press V to toggle verbose mode.")
				input := term.NewNonBlockingStdin()
				defer input.Close() // abort the read loop
				go func() {
					toggleCount := 0
					var b [1]byte
					for {
						if _, err := input.Read(b[:]); err != nil {
							return // exit goroutine
						}
						switch b[0] {
						case 3: // Ctrl-C
							cancel() // cancel the tail context
						case 10, 13: // Enter or Return
							fmt.Println(" ") // empty line, but overwrite the spinner
						case 'v', 'V':
							verbose := !options.Verbose
							options.Verbose = verbose
							modeStr := "OFF"
							if verbose {
								modeStr = "ON"
							}
							if toggleCount++; toggleCount == 4 && !verbose {
								modeStr += ". I like the way you work it, no verbosity."
							}
							term.Info("Verbose mode", modeStr)
							track.Evt("Verbose Toggled", P("verbose", verbose), P("toggleCount", toggleCount))
						}
					}
				}()
			}
		}
	}

	return receiveLogs(ctx, provider, projectName, tailRequest, serverStream, options, doSpinner, handler, cancel)
}

func receiveLogs(ctx context.Context, provider client.Provider, projectName string, tailRequest *defangv1.TailRequest, serverStream client.ServerStream[defangv1.TailResponse], options TailOptions, doSpinner bool, handler LogEntryHandler, cancel context.CancelFunc) error {
	skipDuplicate := false
	var err error
	for {
		if !serverStream.Receive() {
			if errors.Is(serverStream.Err(), context.Canceled) || errors.Is(serverStream.Err(), context.DeadlineExceeded) {
				return &CancelError{TailOptions: options, error: serverStream.Err(), ProjectName: projectName}
			}

			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if isTransientError(serverStream.Err()) {
				term.Debug("Disconnected:", serverStream.Err())
				var spaces int
				if !options.Raw {
					spaces, _ = term.Warnf("Reconnecting...\r") // overwritten below
				}
				if err := provider.DelayBeforeRetry(ctx); err != nil {
					return err
				}
				tailRequest.Since = timestamppb.New(options.Since)
				serverStream, err = provider.QueryLogs(ctx, tailRequest)
				if err != nil {
					term.Debug("Reconnect failed:", err)
					return err
				}
				if !options.Raw {
					term.Printf("%*s", spaces, "\r") // clear the "reconnecting" message
				}
				skipDuplicate = true
				continue
			}

			return serverStream.Err() // returns nil on EOF
		}
		msg := serverStream.Msg()

		if msg == nil {
			continue
		}

		err := handleMsgEntries(msg, &options, doSpinner, skipDuplicate, handler)
		if err != nil {
			cancel() // TODO: stuck on defer Close() if we don't do this
			return err
		}
	}
}

func handleMsgEntries(msg *defangv1.TailResponse, options *TailOptions, doSpinner bool, skipDuplicate bool, handler func(*defangv1.LogEntry, *TailOptions) error) error {
	for _, e := range msg.Entries {
		// Replace service progress messages with our own spinner
		if doSpinner && isProgressDot(e.Message) {
			continue
		}
		ts := e.Timestamp.AsTime()
		// Skip duplicate logs (e.g. after reconnecting we might get the same log once more)
		if skipDuplicate && ts.Equal(options.Since) {
			skipDuplicate = false
			continue
		}
		e.Service = valueOrDefault(e.Service, msg.Service)
		e.Host = valueOrDefault(e.Host, msg.Host)
		e.Etag = valueOrDefault(e.Etag, msg.Etag)
		host := e.Host
		service := e.Service

		// HACK: skip noisy CI/CD logs (except errors)
		isInternal := service == "cd" || service == "kaniko" || service == "fabric" || host == "kaniko" || host == "fabric" || host == "ecs" || host == "cloudbuild" || host == "pulumi"
		onlyErrors := !options.Verbose && isInternal
		if onlyErrors && !e.Stderr {
			if options.EndEventDetectFunc != nil {
				if err := options.EndEventDetectFunc(e); err != nil {
					return err
				}
			}
			continue
		}

		if ts.After(options.Since) {
			options.Since = ts
		}
		err := handler(e, options)
		if err != nil {
			term.Debug("Ending tail loop", err)
			return err
		}
	}

	return nil
}

func logEntryPrintHandler(e *defangv1.LogEntry, options *TailOptions) error {
	if options.Raw {
		if e.Stderr {
			term.Error(e.Message)
		} else {
			term.Println(e.Message)
		}
		return nil
	}

	ts := e.Timestamp.AsTime()
	tsString := ts.Local().Format(RFC3339Milli)
	tsColor := termenv.ANSIBrightBlack
	if term.HasDarkBackground() {
		tsColor = termenv.ANSIWhite
	}
	if e.Stderr {
		tsColor = termenv.ANSIBrightRed
	}
	var prefixLen int
	trimmed := strings.TrimRight(e.Message, "\t\r\n ")
	buf := term.NewMessageBuilder(term.StdoutCanColor())
	for i, line := range strings.Split(trimmed, "\n") {
		if i == 0 {
			prefixLen, _ = buf.Printc(tsColor, tsString, " ")
			if options.Deployment == "" {
				l, _ := buf.Printc(termenv.ANSIYellow, e.Etag, " ")
				prefixLen += l
			}
			if len(options.Services) != 1 {
				l, _ := buf.Printc(termenv.ANSIGreen, e.Service, " ")
				prefixLen += l
			}
			if options.Verbose {
				l, _ := buf.Printc(termenv.ANSIMagenta, e.Host, " ")
				prefixLen += l
			}
		} else {
			buf.WriteString(strings.Repeat(" ", prefixLen))
		}
		if term.StdoutCanColor() {
			if !strings.Contains(line, "\033[") {
				line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
			}
		} else {
			line = term.StripAnsi(line)
		}
		buf.WriteString(line)
		buf.WriteRune('\n')
	}
	term.Print(buf.String())

	// Detect end logging event
	if options.EndEventDetectFunc != nil {
		if err := options.EndEventDetectFunc(e); err != nil {
			return err
		}
	}
	return nil
}

func valueOrDefault(value, def string) string {
	if value != "" {
		return value
	}
	return def
}

func isProgressDot(line string) bool {
	if line == "" || line == "." {
		return true
	}
	stripped := term.StripAnsi(line)
	return strings.TrimLeft(stripped, ".") == ""
}
