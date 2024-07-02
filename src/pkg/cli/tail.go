package cli

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	tea "github.com/charmbracelet/bubbletea"
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

type TailDetectStopEventFunc func(services []string, host string, eventlog string) bool

type TailOptions struct {
	Services           []string
	Etag               types.ETag
	Since              time.Time
	Raw                bool
	EndEventDetectFunc TailDetectStopEventFunc
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

type model struct {
	// client         client.Client
	serverStream   client.ServerStream[defangv1.TailResponse]
	logs           []*defangv1.LogEntry
	showTimestamps bool
	verbose        bool
	// skipDuplicate  bool
	err         error
	showEtag    bool
	showService bool
	// height         int
}

func (m model) Init() tea.Cmd {
	return m.waitForEvents()
}

func (m model) waitForEvents() tea.Cmd {
	return func() tea.Msg {
		if !m.serverStream.Receive() {
			if errors.Is(m.serverStream.Err(), context.Canceled) {
				m.err = &CancelError{error: m.serverStream.Err()}
				return tea.Quit
			}
			// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
			// code := connect.CodeOf(m.serverStream.Err())
			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
			// if code == connect.CodeUnavailable || (code == connect.CodeInternal && !connect.IsWireError(m.serverStream.Err())) {
			// 	term.Debug("Disconnected:", m.serverStream.Err())
			// 	serverStream, err := m.client.Tail(ctx, &defangv1.TailRequest{Services: params.Services, Etag: params.Etag, Since: timestamppb.New(params.Since)})
			// 	if err != nil {
			// 		term.Debug("Reconnect failed:", err)
			// 		return tea.Quit
			// 	}
			// 	m.serverStream = serverStream
			// }
			return tea.Quit
		}
		return tailResponse(m.serverStream.Msg())
	}
}

type tailResponse *defangv1.TailResponse

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "t":
			m.showTimestamps = !m.showTimestamps
		case "v":
			m.verbose = !m.verbose
		case "e":
			m.showEtag = !m.showEtag
		case "s":
			m.showService = !m.showService
		case "q", "esc":
			return m, tea.Quit
		default:
			if msg.Type == tea.KeyCtrlC {
				return m, tea.Quit
			}
		}
	case tailResponse:
		if msg == nil {
			return m, m.waitForEvents()
		}
		for _, pe := range msg.Entries {
			if pe.Service == "" {
				pe.Service = msg.Service
			}
			if pe.Host == "" {
				pe.Host = msg.Host
			}
			if pe.Etag == "" {
				pe.Etag = msg.Etag
			}
		}
		m.logs = append(m.logs, msg.Entries...)
		return m, m.waitForEvents()
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	for _, e := range m.logs {
		prefixLen := 0
		trimmed := strings.TrimRight(e.Message, "\t\r\n ")
		for i, line := range strings.Split(trimmed, "\n") {
			if i == 0 {
				if m.showTimestamps {
					prefixLen, _ = b.WriteString(e.Timestamp.AsTime().Local().Format(RFC3339Micro))
					b.WriteByte(' ')
					prefixLen++ // space
				}
				if m.showEtag {
					b.WriteString(e.Etag)
					b.WriteByte(' ')
					prefixLen += len(e.Etag) + 1
				}
				if m.showService {
					b.WriteString(e.Service)
					b.WriteByte(' ')
					prefixLen += len(e.Service) + 1
				}
				if m.verbose {
					b.WriteString(e.Host)
					b.WriteByte(' ')
					prefixLen += len(e.Host) + 1
				}
			} else {
				b.WriteString(strings.Repeat(" ", prefixLen))
			}
			// if term.StdoutCanColor() {
			// 	if !strings.Contains(line, "\033[") {
			// 		line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
			// 	}
			// 	term.Reset()
			// } else {
			// 	line = term.StripAnsi(line)
			// }
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func tail(ctx context.Context, client client.Client, params TailOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	// defer println("canceled")
	defer cancel()
	// defer println("canceling")

	var since *timestamppb.Timestamp
	if params.Since.IsZero() {
		params.Since = time.Now() // this is used to continue from the last timestamp
	} else {
		since = timestamppb.New(params.Since)
	}
	serverStream, err := client.Tail(ctx, &defangv1.TailRequest{Services: params.Services, Etag: params.Etag, Since: since})
	if err != nil {
		return err
	}
	defer println("closed")
	defer serverStream.Close() // this works because it takes a pointer receiver
	defer println("closing")

	if term.IsTerminal() && !params.Raw {
		if !DoVerbose {
			// Allow the user to toggle verbose mode with the V key
			term.Info("Press V to toggle verbose mode")
		}
	}

	initialModel := model{
		serverStream:   serverStream,
		logs:           []*defangv1.LogEntry{},
		showTimestamps: true,
		verbose:        DoVerbose,
		showEtag:       params.Etag == "",
		showService:    len(params.Services) != 1,
	}
	p := tea.NewProgram(initialModel, tea.WithContext(ctx))
	_, err = p.Run()
	cancel()
	println("asdf")
	return err
	/*
	   skipDuplicate := false

	   	for {
	   		if !serverStream.Receive() {
	   			if errors.Is(serverStream.Err(), context.Canceled) {
	   				return &CancelError{Services: params.Services, Etag: params.Etag, Last: params.Since, error: serverStream.Err()}
	   			}

	   			// TODO: detect ALB timeout (504) or Fabric restart and reconnect automatically
	   			code := connect.CodeOf(serverStream.Err())
	   			// Reconnect on Error: internal: stream error: stream ID 5; INTERNAL_ERROR; received from peer
	   			if code == connect.CodeUnavailable || (code == connect.CodeInternal && !connect.IsWireError(serverStream.Err())) {
	   				term.Debug("Disconnected:", serverStream.Err())
	   				var spaces int
	   				if !params.Raw {
	   					spaces, _ = term.Warnf("Reconnecting...\r") // overwritten below
	   				}
	   				pkg.SleepWithContext(ctx, 1*time.Second)
	   				serverStream, err = client.Tail(ctx, &defangv1.TailRequest{Services: params.Services, Etag: params.Etag, Since: timestamppb.New(params.Since)})
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

	   			var prefixLen int
	   			trimmed := strings.TrimRight(e.Message, "\t\r\n ")
	   			for i, line := range strings.Split(trimmed, "\n") {
	   				if i == 0 {
	   					tsString := ts.Local().Format(RFC3339Micro)
	   					tsColor := termenv.ANSIBrightBlack
	   					if term.HasDarkBackground() {
	   						tsColor = termenv.ANSIWhite
	   					}
	   					if e.Stderr {
	   						tsColor = termenv.ANSIBrightRed
	   					}
	   					prefixLen, _ = term.Printc(tsColor, tsString, " ")
	   					if params.Etag == "" {
	   						l, _ := term.Printc(termenv.ANSIYellow, etag, " ")
	   						prefixLen += l
	   					}
	   					if len(params.Services) != 1 {
	   						l, _ := term.Printc(termenv.ANSIGreen, service, " ")
	   						prefixLen += l
	   					}
	   					if DoVerbose {
	   						l, _ := term.Printc(termenv.ANSIMagenta, host, " ")
	   						prefixLen += l
	   					}
	   				} else {
	   					term.Print(strings.Repeat(" ", prefixLen))
	   				}
	   				if term.StdoutCanColor() {
	   					if !strings.Contains(line, "\033[") {
	   						line = colorKeyRegex.ReplaceAllString(line, replaceString) // add some color
	   					}
	   					term.Reset()
	   				} else {
	   					line = term.StripAnsi(line)
	   				}
	   				term.Println(line)

	   				// Detect end logging event
	   				if params.EndEventDetectFunc != nil && params.EndEventDetectFunc([]string{service}, host, line) {
	   					cancel()
	   					return nil
	   				}
	   			}
	   		}
	   	}
	*/
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
