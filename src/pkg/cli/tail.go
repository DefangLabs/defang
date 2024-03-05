package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/muesli/termenv"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ansiCyan        = "\033[36m"
	ansiReset       = "\033[0m"
	replaceString   = ansiCyan + "$0" + ansiReset
	spinner         = `-\|/`
	TimestampFormat = "15:04:05.000000 "
)

var (
	colorKeyRegex = regexp.MustCompile(`"(?:\\["\\/bfnrt]|[^\x00-\x1f"\\]|\\u[0-9a-fA-F]{4})*"\s*:|[^\x00-\x20"=&?]+=`) // handles JSON, logfmt, and query params
)

type P = client.Property // shorthand for tracking properties

// ParseTimeOrDuration parses a time string or duration string (e.g. 1h30m) and returns a time.Time.
// At a minimum, this function supports RFC3339Nano, Go durations, and our own TimestampFormat (local).
func ParseTimeOrDuration(str string) (time.Time, error) {
	if strings.ContainsAny(str, "TZ") {
		return time.Parse(time.RFC3339Nano, str)
	}
	if strings.Contains(str, ":") {
		local, err := time.ParseInLocation("15:04:05.999999", str, time.Local)
		if err != nil {
			return time.Time{}, err
		}
		// Replace the year, month, and day of t with today's date
		now := time.Now().Local()
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
	return time.Now().Add(-dur), nil // - because we want to go back in time
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

func Tail(ctx context.Context, client client.Client, service, etag string, since time.Time, raw bool) error {
	if service != "" {
		service = NormalizeServiceName(service)
		// Show a warning if the service doesn't exist (yet);; TODO: could do fuzzy matching and suggest alternatives
		if _, err := client.Get(ctx, &v1.ServiceID{Name: service}); err != nil {
			switch connect.CodeOf(err) {
			case connect.CodeNotFound:
				Warn(" ! Service does not exist (yet):", service)
			case connect.CodeUnknown:
				// Ignore unknown (nil) errors
			default:
				Warn(" !", err)
			}
		}
	}

	if DoDryRun {
		return ErrDryRun
	}

	tailClient, err := client.Tail(ctx, &v1.TailRequest{Service: service, Etag: etag, Since: timestamppb.New(since)})
	if err != nil {
		return err
	}
	defer tailClient.Close() // this works because it takes a pointer receiver

	spinMe := 0
	doSpinner := !raw && canColor && IsTerminal

	if IsTerminal && !raw {
		if doSpinner {
			stdout.HideCursor()
			defer stdout.ShowCursor()
		}

		if !DoVerbose {
			Info(" * Press V to toggle verbose mode")
			oldState, err := term.MakeUnbuf(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			defer term.Restore(int(os.Stdin.Fd()), oldState)

			input := term.NewNonBlockingStdin()
			defer input.Close() // abort the read
			go func() {
				var b [1]byte
				for {
					if _, err := input.Read(b[:]); err != nil {
						return // exit goroutine
					}
					switch b[0] {
					case 10, 13: // Enter or Return
						Println(Nop, " ") // empty line, but overwrite the spinner
					case 'v', 'V':
						verbose := !DoVerbose
						DoVerbose = verbose
						modeStr := "off"
						if verbose {
							modeStr = "on"
						}
						Info(" * Verbose mode", modeStr)
						go client.Track("Verbose Toggled", P{"verbose", verbose})
					}
				}
			}()
		}
	}

	timestampZone := time.Local
	timestampFormat := TimestampFormat
	if time.Since(since) >= 24*time.Hour {
		timestampFormat = "2006-01-02T15:04:05.000000Z " // like RFC3339Nano but with 6 digits of precision
		timestampZone = time.UTC
	}

	skipDuplicate := false
	for {
		if !tailClient.Receive() {
			if errors.Is(tailClient.Err(), context.Canceled) {
				return &CancelError{Service: service, Etag: etag, Last: since, error: tailClient.Err()}
			}

			// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
			code := connect.CodeOf(tailClient.Err())
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			if code == connect.CodeUnavailable || (code == connect.CodeInternal && !connect.IsWireError(tailClient.Err())) {
				Debug(" - Disconnected:", tailClient.Err())
				if !raw {
					Fprint(stderr, WarnColor, " ! Reconnecting...\r") // overwritten below
				}
				time.Sleep(time.Second)
				tailClient, err = client.Tail(ctx, &v1.TailRequest{Service: service, Etag: etag, Since: timestamppb.New(since)})
				if err != nil {
					Debug(" - Reconnect failed:", err)
					return err
				}
				if !raw {
					Fprintln(stderr, WarnColor, " ! Reconnected!   ")
				}
				skipDuplicate = true
				continue
			}

			return tailClient.Err() // returns nil on EOF
		}
		msg := tailClient.Msg()

		// Show a spinner if we're not in raw mode and have a TTY
		if doSpinner {
			fmt.Printf("\r%c\r", spinner[spinMe%len(spinner)])
			spinMe++
		}

		// HACK: skip noisy CI/CD logs (except errors)
		isInternal := msg.Service == "cd" || msg.Service == "ci" || msg.Service == "kaniko" || msg.Service == "fabric"
		onlyErrors := !DoVerbose && isInternal
		for _, e := range msg.Entries {
			if onlyErrors && !e.Stderr {
				continue
			}

			ts := e.Timestamp.AsTime()
			if skipDuplicate && ts.Equal(since) {
				skipDuplicate = false
				continue
			}
			if ts.After(since) {
				since = ts
			}

			if raw {
				out := stdout
				if e.Stderr {
					out = stderr
				}
				Fprintln(out, Nop, e.Message) // TODO: trim trailing newline because we're already printing one?
				continue
			}

			// Replace service progress messages with our own spinner
			if doSpinner && isProgressDot(e.Message) {
				continue
			}

			tsString := ts.In(timestampZone).Format(timestampFormat)
			tsColor := termenv.ANSIWhite
			if e.Stderr {
				tsColor = termenv.ANSIBrightRed
			}
			var prefixLen int
			trimmed := strings.TrimRight(e.Message, "\t\r\n ")
			for i, line := range strings.Split(trimmed, "\n") {
				if i == 0 {
					prefixLen, _ = Print(tsColor, tsString)
					if etag == "" {
						l, _ := Print(termenv.ANSIYellow, msg.Etag, " ")
						prefixLen += l
					}
					if service == "" {
						l, _ := Print(termenv.ANSIGreen, msg.Service, " ")
						prefixLen += l
					}
					if DoVerbose {
						l, _ := Print(termenv.ANSIMagenta, msg.Host, " ")
						prefixLen += l
					}
				} else {
					Print(Nop, strings.Repeat(" ", prefixLen))
				}
				if doColor(stdout) {
					if !strings.Contains(line, "\033[") {
						line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
					}
					stdout.Reset()
				} else {
					line = pkg.StripAnsi(line)
				}
				Println(Nop, line)
			}
		}
	}
}

func isProgressDot(line string) bool {
	if len(line) <= 1 {
		return true
	}
	stripped := pkg.StripAnsi(line)
	for _, r := range stripped {
		if r != '.' {
			return false
		}
	}
	return true
}
