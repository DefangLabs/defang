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
	"github.com/defang-io/defang/src/pkg/term"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	replaceString   = string(Cyan + "$0" + Reset)
	spinner         = `-\|/`
	TimestampFormat = "15:04:05.000000 "
)

var (
	colorKeyRegex = regexp.MustCompile(`"(?:\\["\\/bfnrt]|[^\x00-\x1f"\\]|\\u[0-9a-fA-F]{4})*"\s*:|[^\x00-\x20"=&?]+=`) // handles JSON, logfmt, and query params
)

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

func Tail(ctx context.Context, client defangv1connect.FabricControllerClient, service, etag string, since time.Time, raw bool) error {
	if DoDryRun {
		return nil
	}

	if service != "" {
		service = NormalizeServiceName(service)
		// Show a warning if the service doesn't exist (yet); TODO: could do fuzzy matching and suggest alternatives
		if _, err := client.Get(ctx, connect.NewRequest(&v1.ServiceID{Name: service})); err != nil {
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

	tailClient, err := client.Tail(ctx, connect.NewRequest(&v1.TailRequest{Service: service, Etag: etag, Since: timestamppb.New(since)}))
	if err != nil {
		return err
	}
	defer tailClient.Close() // this works because it takes a pointer receiver

	if IsTerminal && !raw {
		if CanColor && DoColor {
			fmt.Print(HideCursor)
			defer fmt.Print(Reset + ShowCursor)
		}

		if !DoVerbose {
			Info(" * Press V to toggle verbose mode")
			oldState, err := term.MakeUnbuf(int(os.Stdin.Fd()))
			if err != nil {
				return err
			}
			defer term.Restore(int(os.Stdin.Fd()), oldState)

			defer os.Stdin.Close() // abort the read
			go func() {
				var b [1]byte
				for {
					if _, err := os.Stdin.Read(b[:]); err != nil {
						return
					}
					if b[0] == 'V' || b[0] == 'v' {
						verbose := !DoVerbose
						DoVerbose = verbose
						state := "off"
						if verbose {
							state = "on"
						}
						Info(" * Verbose mode", state)
					}
				}
			}()
		}
	}

	// colorizer := colorizer{}
	spinMe := 0
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
					Fprint(os.Stderr, WarnColor, " ! Reconnecting...\r") // overwritten below
				}
				time.Sleep(time.Second)
				tailClient, err = client.Tail(ctx, connect.NewRequest(&v1.TailRequest{Service: service, Etag: etag, Since: timestamppb.New(since)}))
				if err != nil {
					Debug(" - Reconnect failed:", err)
					return err
				}
				if !raw {
					Fprintln(os.Stderr, WarnColor, " ! Reconnected!   ")
				}
				skipDuplicate = true
				continue
			}

			return tailClient.Err() // returns nil on EOF
		}
		msg := tailClient.Msg()

		// Show a spinner if we're not in raw mode and have a TTY
		if !raw && DoColor {
			fmt.Printf("%c\r", spinner[spinMe%len(spinner)])
			spinMe++
			// Replace service progress messages with our own spinner
			if isProgressMsg(msg.Entries) {
				continue
			}
		}

		isInternal := !strings.HasPrefix(msg.Host, "ip-") // FIXME: not true for BYOC
		for _, e := range msg.Entries {
			if !DoVerbose && !e.Stderr && isInternal {
				// HACK: skip noisy CI/CD logs (except errors)
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
				out := os.Stdout
				if e.Stderr {
					out = os.Stderr
				}
				Fprintln(out, Nop, e.Message) // TODO: trim trailing newline because we're already printing one?
				continue
			}

			tsString := ts.In(timestampZone).Format(timestampFormat)
			tsColor := Bright
			if e.Stderr {
				tsColor = BrightRed
			}
			var prefixLen int
			trimmed := strings.TrimRight(e.Message, "\t\r\n ")
			for i, line := range strings.Split(trimmed, "\n") {
				if i == 0 {
					prefixLen, _ = Print(tsColor, tsString)
					if etag == "" {
						l, _ := Print(Yellow, msg.Etag, " ")
						prefixLen += l
					}
					if service == "" {
						l, _ := Print(Green, msg.Service, " ")
						prefixLen += l
					}
					if DoVerbose {
						l, _ := Print(Purple, msg.Host, " ")
						prefixLen += l
					}
				} else {
					Print(Nop, strings.Repeat(" ", prefixLen))
				}
				if DoColor {
					if !strings.Contains(line, "\033[") {
						line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
					}
				} else {
					line = StripAnsi(line)
				}
				Println(Reset, line)
			}
		}
	}
}

func isProgressDot(line string) bool {
	return len(line) <= 1 || len(StripAnsi(line)) <= 1
}

func isProgressMsg(entries []*v1.LogEntry) bool {
	return len(entries) == 0 || (len(entries) == 1 && isProgressDot(entries[0].Message))
}
