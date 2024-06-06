package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/spinner"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/muesli/termenv"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ansiCyan      = "\033[36m"
	ansiReset     = "\033[0m"
	replaceString = ansiCyan + "$0" + ansiReset
	RFC3339Micro  = "2006-01-02T15:04:05.000000Z07:00" // like RFC3339Nano but with 6 digits of precision
)

var (
	colorKeyRegex = regexp.MustCompile(`"(?:\\["\\/bfnrt]|[^\x00-\x1f"\\]|\\u[0-9a-fA-F]{4})*"\s*:|[^\x00-\x20"=&?]+=`) // handles JSON, logfmt, and query params
	DoVerbose     = false
)

type ServiceStatus string

const (
	ServiceDeploymentStarting   ServiceStatus = "STARTING"
	ServiceDeploymentInProgress ServiceStatus = "IN_PROGRESS"
	ServiceStarted              ServiceStatus = "COMPLETED"
	ServiceStopping             ServiceStatus = "STOPPING"
	ServiceStopped              ServiceStatus = "STOPPED"
	ServiceDeactivating         ServiceStatus = "DEACTIVATING"
	ServiceDeprovisioning       ServiceStatus = "DEPROVISIONING"
	ServiceFailed               ServiceStatus = "FAILED"
	ServiceUnknown              ServiceStatus = "UNKNOWN"
)

type EndLogConditional struct {
	Service  string
	Host     string
	EventLog string
}

type TailDetectStopEventFunc func(service string, host string, eventlog string) bool

type TailOptions struct {
	Service            string
	Etag               types.ETag
	Since              time.Time
	Raw                bool
	EndEventDetectFunc TailDetectStopEventFunc
}

type P = client.Property // shorthand for tracking properties

func CreateEndLogEventDetectFunc(conditionals []EndLogConditional) TailDetectStopEventFunc {
	return func(service string, host string, eventLog string) bool {
		for _, conditional := range conditionals {
			if service == "" || service == conditional.Service {
				if host == "" || host == conditional.Host {
					if strings.Contains(eventLog, conditional.EventLog) {
						return true
					}
				}
			}
		}
		return false
	}
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
	Service string
	Etag    string
	Last    time.Time
	error
}

func (cerr *CancelError) Error() string {
	cmd := "tail --since " + cerr.Last.UTC().Format(time.RFC3339Nano)
	if cerr.Service != "" {
		cmd += " --name " + cerr.Service
	}
	if cerr.Etag != "" {
		cmd += " --etag " + cerr.Etag
	}
	if DoVerbose {
		cmd += " --verbose"
	}
	return cmd
}

func (cerr *CancelError) Unwrap() error {
	return cerr.error
}

func Tail(ctx context.Context, client client.Client, params TailOptions) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}
	term.Debug(" - Tailing logs in project", projectName)

	if params.Service != "" {
		params.Service = NormalizeServiceName(params.Service)
		// Show a warning if the service doesn't exist (yet); TODO: could do fuzzy matching and suggest alternatives
		if _, err := client.GetService(ctx, &defangv1.ServiceID{Name: params.Service}); err != nil {
			switch connect.CodeOf(err) {
			case connect.CodeNotFound:
				term.Warn(" ! Service does not exist (yet):", params.Service)
			case connect.CodeUnknown:
				// Ignore unknown (nil) errors
			default:
				term.Warn(" !", err)
			}
		}
	}

	if DoDryRun {
		return ErrDryRun
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var since *timestamppb.Timestamp
	if !params.Since.IsZero() {
		since = timestamppb.New(params.Since)
	}
	serverStream, err := client.Tail(ctx, &defangv1.TailRequest{Service: params.Service, Etag: params.Etag, Since: since})
	if err != nil {
		return err
	}
	defer serverStream.Close() // this works because it takes a pointer receiver

	spin := spinner.New()
	doSpinner := !params.Raw && term.CanColor && term.IsTerminal

	if term.IsTerminal && !params.Raw {
		if doSpinner {
			term.Stdout.HideCursor()
			defer term.Stdout.ShowCursor()
		}

		if !DoVerbose {
			// Allow the user to toggle verbose mode with the V key
			if oldState, err := term.MakeUnbuf(int(os.Stdin.Fd())); err == nil {
				defer term.Restore(int(os.Stdin.Fd()), oldState)

				term.Info(" * Press V to toggle verbose mode")
				input := term.NewNonBlockingStdin()
				defer input.Close() // abort the read loop
				go func() {
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
							verbose := !DoVerbose
							DoVerbose = verbose
							modeStr := "OFF"
							if verbose {
								modeStr = "ON"
							}
							term.Info(" * Verbose mode", modeStr)
							go client.Track("Verbose Toggled", P{"verbose", verbose})
						}
					}
				}()
			}
		}
	}

	skipDuplicate := false
	for {
		if !serverStream.Receive() {
			if errors.Is(serverStream.Err(), context.Canceled) {
				return &CancelError{Service: params.Service, Etag: params.Etag, Last: params.Since, error: serverStream.Err()}
			}

			// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
			code := connect.CodeOf(serverStream.Err())
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if code == connect.CodeUnavailable || (code == connect.CodeInternal && !connect.IsWireError(serverStream.Err())) {
				term.Debug(" - Disconnected:", serverStream.Err())
				var spaces int
				if !params.Raw {
					spaces, _ = term.Fprint(term.Stderr, term.WarnColor, " ! Reconnecting...\r") // overwritten below
				}
				pkg.SleepWithContext(ctx, 1*time.Second)
				serverStream, err = client.Tail(ctx, &defangv1.TailRequest{Service: params.Service, Etag: params.Etag, Since: timestamppb.New(params.Since)})
				if err != nil {
					term.Debug(" - Reconnect failed:", err)
					return err
				}
				if !params.Raw {
					fmt.Fprintf(term.Stderr, "%*s", spaces, "\r") // clear the "reconnecting" message
				}
				skipDuplicate = true
				continue
			}

			return serverStream.Err() // returns nil on EOF
		}
		msg := serverStream.Msg()

		// Show a spinner if we're not in raw mode and have a TTY
		if doSpinner {
			fmt.Print(spin.Next())
		}

		// HACK: skip noisy CI/CD logs (except errors)
		isInternal := msg.Service == "cd" || msg.Service == "ci" || msg.Service == "kaniko" || msg.Service == "fabric" || msg.Host == "kaniko" || msg.Host == "fabric"
		onlyErrors := !DoVerbose && isInternal
		for _, e := range msg.Entries {
			if onlyErrors && !e.Stderr {
				if params.EndEventDetectFunc != nil && params.EndEventDetectFunc(msg.Service, msg.Host, e.Message) {
					return nil
				}
				continue
			}

			ts := e.Timestamp.AsTime()
			if skipDuplicate && ts.Equal(params.Since) {
				skipDuplicate = false
				continue
			}
			if ts.After(params.Since) {
				params.Since = ts
			}

			if params.Raw {
				out := term.Stdout
				if e.Stderr {
					out = term.Stderr
				}
				fmt.Fprintln(out, e.Message) // TODO: trim trailing newline because we're already printing one?
				continue
			}

			// Replace service progress messages with our own spinner
			if doSpinner && isProgressDot(e.Message) {
				continue
			}

			tsString := ts.Local().Format(RFC3339Micro)
			tsColor := termenv.ANSIBrightBlack
			if term.Stdout.HasDarkBackground() {
				tsColor = termenv.ANSIWhite
			}
			if e.Stderr {
				tsColor = termenv.ANSIBrightRed
			}
			var prefixLen int
			trimmed := strings.TrimRight(e.Message, "\t\r\n ")
			for i, line := range strings.Split(trimmed, "\n") {
				if i == 0 {
					prefixLen, _ = term.Print(tsColor, tsString, " ")
					if params.Etag == "" {
						l, _ := term.Print(termenv.ANSIYellow, msg.Etag, " ")
						prefixLen += l
					}
					if params.Service == "" {
						l, _ := term.Print(termenv.ANSIGreen, msg.Service, " ")
						prefixLen += l
					}
					if DoVerbose {
						l, _ := term.Print(termenv.ANSIMagenta, msg.Host, " ")
						prefixLen += l
					}
				} else {
					fmt.Print(strings.Repeat(" ", prefixLen))
				}
				if term.CanColor {
					if !strings.Contains(line, "\033[") {
						line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
					}
					term.Stdout.Reset()
				} else {
					line = term.StripAnsi(line)
				}
				fmt.Println(line)

				// Detect end logging event
				if params.EndEventDetectFunc != nil && params.EndEventDetectFunc(msg.Service, msg.Host, line) {
					cancel()
					return nil
				}
			}
		}
	}
}

func isProgressDot(line string) bool {
	if line == "" || line == "." {
		return true
	}
	stripped := term.StripAnsi(line)
	return strings.TrimLeft(stripped, ".") == ""
}
