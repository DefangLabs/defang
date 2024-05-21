package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/defang-io/defang/src/pkg"
	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/spinner"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
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

type TailDetectStopEventFunc func(service string, host string, eventlog string) bool

type TailOptions struct {
	Service            string
	Etags              []types.ETag
	Since              time.Time
	Raw                bool
	EndEventDetectFunc TailDetectStopEventFunc
}

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
	Etags   []types.ETag
	Last    time.Time
	error
}

func (cerr *CancelError) Error() string {
	cmd := "tail --since " + cerr.Last.UTC().Format(time.RFC3339Nano)
	if cerr.Service != "" {
		cmd += " --name " + cerr.Service
	}
	if len(cerr.Etags) > 0 {
		cmd += " --etag " + strings.Join(cerr.Etags, " ")
	}
	if DoVerbose {
		cmd += " --verbose"
	}
	return cmd
}

func (cerr *CancelError) Unwrap() error {
	return cerr.error
}

func warnOnServiceNotFound(ctx context.Context, client client.Client, params TailOptions) {
	// Show a warning if the service doesn't exist (yet);; TODO: could do fuzzy matching and suggest alternatives
	if _, err := client.Get(ctx, &defangv1.ServiceID{Name: params.Service}); err != nil {
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

func handleUserInput(client client.Client, cancel context.CancelFunc) {
	input := term.NewNonBlockingStdin()
	defer input.Close() // abort the read loop

	var b [1]byte
	for {
		if _, err := input.Read(b[:]); err != nil {
			return // exit goroutine
		}
		switch b[0] {
		case 3: // Ctrl-C
			cancel() // cancel the tail context
			return
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
}

type connectionHandlerResponse struct {
	stream        *client.ServerStream[defangv1.TailResponse]
	skipDuplicate bool
	retryable     bool
}

func handleStreamConnectionErrors(ctx context.Context, client client.Client, serverStream client.ServerStream[defangv1.TailResponse], skipDuplicate bool, params TailOptions) (*connectionHandlerResponse, error) {
	response := connectionHandlerResponse{
		stream:        &serverStream,
		skipDuplicate: skipDuplicate,
		retryable:     false,
	}

	if errors.Is(serverStream.Err(), context.Canceled) {
		return &response, &CancelError{Service: params.Service, Etags: params.Etags, Last: params.Since, error: serverStream.Err()}
	}

	// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
	code := connect.CodeOf(serverStream.Err())

	// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
	if code == connect.CodeUnavailable || (code == connect.CodeInternal && !connect.IsWireError(serverStream.Err())) {
		term.Debug(" - Disconnected:", serverStream.Err())
		if !params.Raw {
			term.Fprint(term.Stderr, term.WarnColor, " ! Reconnecting...\r") // overwritten below
		}
		time.Sleep(time.Second)
		etag := ""
		if len(params.Etags) == 0 {
			etag = params.Etags[0]
		}

		newServerStream, err := client.Tail(ctx, &defangv1.TailRequest{Service: params.Service, Etag: etag, Since: timestamppb.New(params.Since)})
		response.stream = &newServerStream
		if err != nil {
			term.Debug(" - Reconnect failed:", err)
			return &response, err
		}
		if !params.Raw {
			term.Fprint(term.Stderr, term.WarnColor, " ! Reconnected!   \r") // overwritten with logs
		}
		response.skipDuplicate = true
		response.retryable = true
		return &response, nil
	}

	return nil, serverStream.Err() // returns nil on EOF
}

func showSpinner(ctx context.Context, interval time.Duration) {
	spin := spinner.New()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Print(spin.Next())
		case <-ctx.Done():
			// Cancel the spinner when the context is canceled
			return
		}
	}
}

func defaultNoDetectStopEventFunc(string, string, string) bool {
	return false
}

func printIfRaw(isRaw bool, e *defangv1.LogEntry) bool {
	if isRaw {
		out := term.Stdout
		if e.Stderr {
			out = term.Stderr
		}
		fmt.Fprintln(out, e.Message) // TODO: trim trailing newline because we're already printing one?
	}

	return isRaw
}

type logConfig struct {
	ETags     []types.ETag
	service   string
	color     term.Color
	timeStamp string
}

func printLogLine(colIndex int, line string, msg *defangv1.TailResponse, logCfg logConfig, prefixLen int) int {
	if colIndex == 0 {
		prefixLen, _ = term.Print(logCfg.color, logCfg.timeStamp, " ")
		if slices.Contains(logCfg.ETags, msg.Etag) || (len(logCfg.ETags) == 1 && logCfg.ETags[0] == "") {
			l, _ := term.Print(termenv.ANSIYellow, msg.Etag, " ")
			prefixLen += l
		}
		if logCfg.service == "" {
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
		line = pkg.StripAnsi(line)
	}

	fmt.Println(line)

	return prefixLen
}

func processLogs(isErrText bool, ts time.Time, message string, msg *defangv1.TailResponse, params TailOptions) bool {
	logCfg := logConfig{}
	logCfg.ETags = params.Etags
	logCfg.timeStamp = ts.Local().Format(RFC3339Micro)
	logCfg.color = termenv.ANSIWhite
	if isErrText {
		logCfg.color = termenv.ANSIBrightRed
	}

	var prefixLen int
	trimmed := strings.TrimRight(message, "\t\r\n ")
	for i, line := range strings.Split(trimmed, "\n") {
		prefixLen = printLogLine(i, line, msg, logCfg, prefixLen)

		// Detect end logging event
		if params.EndEventDetectFunc(msg.Service, msg.Host, line) {
			return false
		}
	}

	return true
}

func Tail(ctx context.Context, client client.Client, params TailOptions) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}

	if params.Service != "" {
		params.Service = NormalizeServiceName(params.Service)
		warnOnServiceNotFound(ctx, client, params)
	}

	if params.EndEventDetectFunc == nil {
		params.EndEventDetectFunc = defaultNoDetectStopEventFunc
	}

	term.Debug(" - Tailing logs in project", projectName)

	if DoDryRun {
		return ErrDryRun
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// if there is only one etag, filter using on the Tail request, otherwise we will filter later in this function
	etag := ""
	matchAnyEtag := false
	if len(params.Etags) == 1 {
		etag = params.Etags[0]
		matchAnyEtag = etag == ""
	}

	serverStream, err := client.Tail(ctx, &defangv1.TailRequest{Service: params.Service, Etag: etag, Since: timestamppb.New(params.Since)})
	if err != nil {
		return err
	}
	defer serverStream.Close() // this works because it takes a pointer receiver

	doSpinner := !params.Raw && term.CanColor && term.IsTerminal

	// Show a spinner if we're not in raw mode and have a TTY
	if doSpinner {
		go showSpinner(ctx, 500 * time.Millisecond)
	}

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
				go handleUserInput(client, cancel)
			}
		}
	}

	skipDuplicate := false
	for {
		if !serverStream.Receive() {
			connResponse, err := handleStreamConnectionErrors(ctx, client, serverStream, skipDuplicate, params)
			if err != nil {
				return err
			}
			skipDuplicate, serverStream = connResponse.skipDuplicate, *connResponse.stream
			if connResponse.retryable {
				continue
			}
		}
		msg := serverStream.Msg()

		//filter by etag or blank etag
		if !matchAnyEtag && !slices.Contains(params.Etags, msg.Etag) {
			continue
		}

		// HACK: skip noisy CI/CD logs (except errors)
		isInternal := msg.Service == "cd" || msg.Service == "ci" || msg.Service == "kaniko" || msg.Service == "fabric" || msg.Host == "kaniko" || msg.Host == "fabric"
		onlyErrors := !DoVerbose && isInternal

		for _, e := range msg.Entries {
			if onlyErrors && !e.Stderr {
				if params.EndEventDetectFunc(msg.Service, msg.Host, e.Message) {
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

			if printIfRaw(params.Raw, e) {
				continue
			}

			// Replace service progress messages with our own spinner
			if doSpinner && isProgressDot(e.Message) {
				continue
			}

			if !processLogs(e.Stderr, ts, e.Message, msg, params) {
				fmt.Println("Exit eventlog condition found.")
				cancel()
				return nil
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
