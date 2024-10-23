package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
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
	ServiceUnspecified ServiceStatus = "UNSPECIFIED"

	// build states
	ServiceBuildQueued       ServiceStatus = "BUILD_QUEUED"
	ServiceBuildProvisioning ServiceStatus = "BUILD_PROVISIONING"
	ServiceBuildPending      ServiceStatus = "BUILD_PENDING"
	ServiceBuildActivating   ServiceStatus = "BUILD_ACTIVATING"
	ServiceBuildRunning      ServiceStatus = "BUILD_RUNNING"
	ServiceBuildDeactivating ServiceStatus = "BUILD_DEACTIVATING" // build completed

	// update states
	ServiceUpdateQueued ServiceStatus = "UPDATE_QUEUED" // queued for deployment

	// deplpyment states
	ServicePending   ServiceStatus = "PENDING"
	ServiceCompleted ServiceStatus = "COMPLETED"
	ServiceFailed    ServiceStatus = "FAILED"
)

type EndLogConditional struct {
	Service  string
	Host     string
	EventLog string
}

type TailDetectStopEventFunc func(services []string, host string, eventlog string) bool

type TailOptions struct {
	Services           []string
	Etag               types.ETag
	Since              time.Time
	Raw                bool
	EndEventDetectFunc TailDetectStopEventFunc
	LogType            defangv1.LogType
}

type P = client.Property // shorthand for tracking properties

func CreateEndLogEventDetectFunc(conditionals []EndLogConditional) TailDetectStopEventFunc {
	return func(services []string, host string, eventLog string) bool {
		for _, conditional := range conditionals {
			for _, service := range services {
				if service == "" || service == conditional.Service {
					if host == "" || host == conditional.Host {
						if strings.Contains(eventLog, conditional.EventLog) {
							return true
						}
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
	Services []string
	Etag     string
	Last     time.Time
	error
}

func (cerr *CancelError) Error() string {
	cmd := "tail --since " + cerr.Last.UTC().Format(time.RFC3339Nano)
	if len(cerr.Services) > 0 {
		cmd += " --name " + strings.Join(cerr.Services, ",")
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
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Tailing logs in project %q", projectName)

	if len(params.Services) > 0 {
		for _, service := range params.Services {
			service = compose.NormalizeServiceName(service)
			// Show a warning if the service doesn't exist (yet); TODO: could do fuzzy matching and suggest alternatives
			if _, err := client.GetService(ctx, &defangv1.ServiceID{Name: service}); err != nil {
				switch connect.CodeOf(err) {
				case connect.CodeNotFound:
					term.Warn("Service does not exist (yet):", service)
				case connect.CodeUnknown:
					// Ignore unknown (nil) errors
				default:
					term.Warn(err) // TODO: use prettyError(…)
				}
			}
		}
	}

	if DoDryRun {
		return ErrDryRun
	}

	return tail(ctx, client, params)
}

func isTransientError(err error) bool {
	// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
	code := connect.CodeOf(err)
	// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
	return code == connect.CodeUnavailable ||
		(code == connect.CodeInternal && !connect.IsWireError(err)) ||
		errors.Is(err, io.ErrUnexpectedEOF)

}

func tail(ctx context.Context, client client.Client, params TailOptions) error {
	var since *timestamppb.Timestamp
	if params.Since.IsZero() {
		params.Since = time.Now() // this is used to continue from the last timestamp
	} else {
		since = timestamppb.New(params.Since)
	}

	serverStream, err := client.Follow(ctx, &defangv1.TailRequest{
		Services: params.Services,
		Etag:     params.Etag,
		Since:    since,
		Type:     params.LogType,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-ctx.Done()
		serverStream.Close() // this works because it takes a pointer receiver
	}()

	spin := spinner.New()
	doSpinner := !params.Raw && term.StdoutCanColor() && term.IsTerminal()

	if term.IsTerminal() && !params.Raw {
		if doSpinner {
			term.HideCursor()
			defer term.ShowCursor()

			cancelSpinner := spin.Start(ctx)
			defer cancelSpinner()
		}

		if !DoVerbose {
			// Allow the user to toggle verbose mode with the V key
			if oldState, err := term.MakeUnbuf(int(os.Stdin.Fd())); err == nil {
				defer term.Restore(int(os.Stdin.Fd()), oldState)

				term.Info("Press V to toggle verbose mode")
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
							term.Info("Verbose mode", modeStr)
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
			if errors.Is(serverStream.Err(), context.Canceled) || errors.Is(serverStream.Err(), context.DeadlineExceeded) {
				return &CancelError{Services: params.Services, Etag: params.Etag, Last: params.Since, error: serverStream.Err()}
			}

			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if isTransientError(serverStream.Err()) {
				term.Debug("Disconnected:", serverStream.Err())
				var spaces int
				if !params.Raw {
					spaces, _ = term.Warnf("Reconnecting...\r") // overwritten below
				}
				pkg.SleepWithContext(ctx, 1*time.Second)
				serverStream, err = client.Follow(ctx, &defangv1.TailRequest{Services: params.Services, Etag: params.Etag, Since: timestamppb.New(params.Since)})
				if err != nil {
					term.Debug("Reconnect failed:", err)
					return err
				}
				if !params.Raw {
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

		for _, e := range msg.Entries {
			service := valueOrDefault(e.Service, msg.Service)
			host := valueOrDefault(e.Host, msg.Host)
			etag := valueOrDefault(e.Etag, msg.Etag)

			// HACK: skip noisy CI/CD logs (except errors)
			isInternal := service == "cd" || service == "ci" || service == "kaniko" || service == "fabric" || host == "kaniko" || host == "fabric"
			onlyErrors := !DoVerbose && isInternal
			if onlyErrors && !e.Stderr {
				if params.EndEventDetectFunc != nil && params.EndEventDetectFunc([]string{service}, host, e.Message) {
					cancel() // TODO: stuck on defer Close() if we don't do this
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
				if e.Stderr {
					term.Error(e.Message)
				} else {
					term.Printlnc(term.InfoColor, e.Message)
				}
				continue
			}

			// Replace service progress messages with our own spinner
			if doSpinner && isProgressDot(e.Message) {
				continue
			}

			tsString := ts.Local().Format(RFC3339Micro)
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
					if params.Etag == "" {
						l, _ := buf.Printc(termenv.ANSIYellow, etag, " ")
						prefixLen += l
					}
					if len(params.Services) == 0 {
						l, _ := buf.Printc(termenv.ANSIGreen, service, " ")
						prefixLen += l
					}
					if DoVerbose {
						l, _ := buf.Printc(termenv.ANSIMagenta, host, " ")
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

				// Detect end logging event
				if params.EndEventDetectFunc != nil && params.EndEventDetectFunc([]string{service}, host, line) {
					cancel() // TODO: stuck on defer Close() if we don't do this
					return nil
				}
			}
			term.Print(buf.String())
		}
	}
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
