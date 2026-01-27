package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/bufbuild/connect-go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestIsProgressDot(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"empty", "", true},
		{"dot", ".", true},
		{"curly", "}", false},
		{"empty line", "\n", false},
		{"ansi dot", "\x1b[1m.\x1b[0m", true},
		{"ansi empty", "\x1b[1m\x1b[0m", true},
		{"pulumi dot", "\033[38;5;3m.\033[0m", true},
		{"pulumi dots", "\033[38;5;3m.\033[0m\033[38;5;3m.\033[0m", true},
		{"not a progress msg", "blah", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProgressDot(tt.line); got != tt.want {
				t.Errorf("isProgressDot() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockTailProvider struct {
	client.Provider
	ServerStreams []client.ServerStream[defangv1.TailResponse]
	Reqs          []*defangv1.TailRequest
}

func (mockTailProvider) DelayBeforeRetry(ctx context.Context) error {
	// No delay for mock
	return ctx.Err()
}

func (m *mockTailProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	dup, _ := proto.Clone(req).(*defangv1.TailRequest)
	m.Reqs = append(m.Reqs, dup)
	if len(m.ServerStreams) == 0 {
		return nil, errors.New("no server stream provided")
	}
	ss := m.ServerStreams[0]
	m.ServerStreams = m.ServerStreams[1:]
	return ss, nil
}

type mockTailStream = client.MockServerStream[defangv1.TailResponse]

func (m *mockTailProvider) MockTimestamp(timestamp time.Time) *mockTailProvider {
	return &mockTailProvider{
		ServerStreams: []client.ServerStream[defangv1.TailResponse]{
			&mockTailStream{
				Resps: []*defangv1.TailResponse{
					{Entries: []*defangv1.LogEntry{
						{Timestamp: timestamppb.New(timestamp)},
					}},
				},
			}, &mockTailStream{Error: io.EOF},
		},
	}
}

func TestTail(t *testing.T) {
	var stdout, stderr bytes.Buffer
	testTerm := term.NewTerm(os.Stdin, &stdout, &stderr)
	testTerm.ForceColor(true)
	defaultTerm := term.DefaultTerm
	term.DefaultTerm = testTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	const projectName = "project1"

	p := &mockTailProvider{
		ServerStreams: []client.ServerStream[defangv1.TailResponse]{
			&mockTailStream{
				Resps: []*defangv1.TailResponse{
					{Service: "service1", Etag: "SOMEETAG", Host: "SOMEHOST", Entries: []*defangv1.LogEntry{
						{Message: "e1msg1", Timestamp: timestamppb.Now()},
						{Message: "e1msg2", Timestamp: timestamppb.Now(), Etag: "SOMEOTHERETAG"},                                              // Test event etag override the response etag
						{Message: "e1msg3", Timestamp: timestamppb.Now(), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST"},                      // override both etag and host
						{Message: "e1msg4", Timestamp: timestamppb.Now(), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST", Service: "service2"}, // override both etag, host and service
						{Message: "e1err1", Timestamp: timestamppb.Now(), Stderr: true},                                                       // Error message should be in stdout too when not raw
					}},
					{Service: "service1", Etag: "SOMEETAG", Host: "SOMEHOST", Entries: []*defangv1.LogEntry{ // Test entry etag does not affect the default values from response
						{Message: "e2err1", Timestamp: timestamppb.Now(), Stderr: true, Etag: "SOMEOTHERETAG"}, // Error message should be in stdout too when not raw
						{Message: "e2msg1", Timestamp: timestamppb.Now(), Etag: "ENTRIES2ETAG"},
						{Message: "e2msg2", Timestamp: timestamppb.Now()},
						{Message: "e2msg3", Timestamp: timestamppb.Now(), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST", Service: "service2"}, // override both etag, host and service
						{Message: "e2msg4", Timestamp: timestamppb.Now()},
					}},
				},
				Error: connect.NewError(connect.CodeInternal, &cwTypes.SessionStreamingException{}), // to test retries
			},
			&mockTailStream{Error: io.EOF},
		},
	}

	err := Tail(t.Context(), p, projectName, TailOptions{Verbose: true, PrintBookends: true}) // Output host
	if err != io.EOF {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	expectedLogs := []string{
		"SOMEETAG service1 SOMEHOST e1msg1\n",
		"SOMEOTHERETAG service1 SOMEHOST e1msg2\n",
		"SOMEOTHERETAG2 service1 SOMEOTHERHOST e1msg3\n",
		"SOMEOTHERETAG2 service2 SOMEOTHERHOST e1msg4\n",
		"SOMEETAG service1 SOMEHOST e1err1\n",
		"SOMEOTHERETAG service1 SOMEHOST e2err1\n",
		"ENTRIES2ETAG service1 SOMEHOST e2msg1\n",
		"SOMEETAG service1 SOMEHOST e2msg2\n",
		"SOMEOTHERETAG2 service2 SOMEOTHERHOST e2msg3\n",
		"SOMEETAG service1 SOMEHOST e2msg4\n",
		"! Reconnecting...\r                           \r",
	}

	got := strings.SplitAfter(stdout.String(), "\n")

	// expect the first line to be a message about fetching older logs
	if !strings.Contains(got[0], "To view older logs, run: `defang logs --until=") {
		t.Errorf("Expected first line to contain 'To view older logs, run: `defang logs --until=', got %q", got[0])
	}

	// Remove the first line which is a hint
	got = got[1:]

	if len(got) != len(expectedLogs) {
		t.Log(got)
		t.Fatalf("Expecting %v lines of log, but got %v", len(expectedLogs), len(got))
	}

	for i, g := range got {
		e := expectedLogs[i]
		g = term.StripAnsi(g)
		if got := strings.SplitN(g, " ", 2)[1]; got != e { // Remove the date from the log entry
			t.Errorf("Tail() = %q, want %q", got, e)
		}
	}

	if stderr.Len() > 0 {
		t.Errorf("Tail() stderr = %v, want empty", stderr.String())
	}

	if p.Reqs[0].Project != projectName {
		t.Errorf("Tail() sent request with project %v, want %v", p.Reqs[0].Project, projectName)
	}
	if p.Reqs[0].LogType != uint32(logs.LogTypeAll) {
		t.Errorf("Tail() sent request with log type %v, want %v", p.Reqs[0].LogType, logs.LogTypeAll)
	}
	if p.Reqs[0].Since != nil {
		t.Errorf("Tail() sent request with since %v, want nil", p.Reqs[0].Since)
	}

	if len(p.Reqs) != 2 {
		t.Errorf("Tail() sent %v requests, want 2", len(p.Reqs))
	}

	// Ensure the second request is the same but with a valid`since` value
	if p.Reqs[1].Project != projectName {
		t.Errorf("Tail() sent request with project %v, want %v", p.Reqs[0].Project, projectName)
	}
	if p.Reqs[1].LogType != uint32(logs.LogTypeAll) {
		t.Errorf("Tail() sent request with log type %v, want %v", p.Reqs[1].LogType, logs.LogTypeAll)
	}
	if p.Reqs[1].Since == nil {
		t.Errorf("Tail() sent request with since nil, want not nil")
	}
}

func setupTestTerminal() (*bytes.Buffer, *bytes.Buffer, func()) {
	var stdout, stderr bytes.Buffer
	testTerm := term.NewTerm(os.Stdin, &stdout, &stderr)
	testTerm.ForceColor(true)
	defaultTerm := term.DefaultTerm
	term.DefaultTerm = testTerm

	// Cleanup function to reset the terminal
	cleanup := func() {
		term.DefaultTerm = defaultTerm
	}

	return &stdout, &stderr, cleanup
}

func TestUTC(t *testing.T) {
	// Setup terminal for local time test
	stdout, stderr, cleanup := setupTestTerminal()
	if stderr.Len() > 0 {
		t.Errorf("Unexpected stderr output: %v", stderr.String())
	}

	t.Cleanup(cleanup)

	format := time.RFC3339Nano

	// Testing local time
	localTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.Local)

	// Create mock data for tail with local time
	const projectName = "project"
	localMock := &mockTailProvider{}
	localMock = localMock.MockTimestamp(localTime)

	// Start the terminal for local time test
	err := Tail(t.Context(), localMock, projectName, TailOptions{Verbose: true, PrintBookends: true}) // Output host
	if err != nil {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	output := stdout.String()
	lines := strings.Split(output, "\n")
	localTimeparse := strings.TrimSpace(term.StripAnsi(lines[1])) // skip first line which is a hint
	convertedLocalTime, err := time.Parse(format, localTimeparse)
	if err != nil {
		t.Error("Error parsing time:", err)
	}

	if !convertedLocalTime.Equal(localTime) {
		t.Errorf("Original local time:%v != parse local time:%v", localTime, convertedLocalTime)
	}

	EnableUTCMode()

	// Create the UTC time object
	utcTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.Local)

	// Setup terminal for UTC time test
	stdout2, stderr, cleanup2 := setupTestTerminal()
	if stderr.Len() > 0 {
		t.Errorf("Unexpected stderr output: %v", stderr.String())
	}

	t.Cleanup(cleanup2)

	// Create new mock data for tail with UTC time
	utcMock := &mockTailProvider{}
	utcMock = utcMock.MockTimestamp(utcTime)

	err = Tail(t.Context(), utcMock, projectName, TailOptions{PrintBookends: true, Verbose: true})
	if err != nil {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	output2 := stdout2.String()
	lines2 := strings.Split(output2, "\n")
	// Parse the time from the terminal for UTC time
	utcTimeParse := strings.TrimSpace(term.StripAnsi(lines2[1])) // skip first line which is a hint
	convertedUTCTime, err := time.Parse(format, utcTimeParse)
	if err != nil {
		t.Error("Error parsing time:", err)
	}

	if !convertedUTCTime.Equal(utcTime) {
		t.Errorf("Original utc time:%v != parse utc time:%v", utcTime, convertedUTCTime)
	}
}

type mockQueryErrorProvider struct {
	client.Provider
	TailStreamError error
}

func (m mockQueryErrorProvider) QueryLogs(ctx context.Context, req *defangv1.TailRequest) (client.ServerStream[defangv1.TailResponse], error) {
	return &mockTailStream{Error: m.TailStreamError}, nil
}

func TestTailError(t *testing.T) {
	const cancelError = "logs --since=2024-01-02T03:04:05Z --verbose=0 --project-name=project"
	tailOptions := TailOptions{
		Since: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}

	tests := []struct {
		name      string
		err       error
		wantError string
	}{
		{"cancel", context.Canceled, cancelError},
		{"timeout", context.DeadlineExceeded, cancelError},
		{"cd task failure", ecs.TaskFailure{Reason: types.TaskStopCodeEssentialContainerExited}, "EssentialContainerExited: "},
		{"eof", io.EOF, "EOF"},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockQueryErrorProvider{
				TailStreamError: tt.err,
			}
			err := Tail(t.Context(), mock, "project", tailOptions)
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("Tail() error = %q, want: %q", err.Error(), tt.wantError)
				}
			} else if tt.wantError != "" {
				t.Errorf("Tail() error = nil, want %q", tt.wantError)
			}
		})
	}
}

func TestTailContext(t *testing.T) {
	const cancelError = "logs --since=2024-01-02T03:04:05Z --verbose=0 --project-name=project"
	tailOptions := TailOptions{
		Since: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
	}
	mock := &mockDeployProvider{}

	tests := []struct {
		name      string
		cause     error
		wantError string
	}{
		{"cancel", context.Canceled, cancelError},
		{"timeout", context.DeadlineExceeded, cancelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			time.AfterFunc(10*time.Millisecond, func() {
				mock.lock.Lock()
				defer mock.lock.Unlock()
				mock.tailStream.Send(nil, tt.cause)
			})
			err := Tail(ctx, mock, "project", tailOptions)
			if err.Error() != tt.wantError {
				t.Errorf("Tail() error = %q, want: %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestPrintHandler(t *testing.T) {
	var testdata = []struct {
		inputfile  string
		outputfile string
	}{
		{
			inputfile:  "testdata/crew_input.jsonl",
			outputfile: "testdata/crew_output.data",
		},
		{
			inputfile:  "testdata/flask_railpack_input.jsonl",
			outputfile: "testdata/flask_railpack_output.data",
		},
	}

	loc, err := time.LoadLocation("America/Vancouver")
	if err != nil {
		t.Fatalf("Failed to load location: %v", err)
	}
	time.Local = loc
	for _, tc := range testdata {
		logEntries := jsonFileToLogEntry(t, tc.inputfile)

		// Create buffers to capture output and set up terminal
		var stdout, stderr bytes.Buffer
		mockTerm := term.NewTerm(os.Stdin, &stdout, &stderr)

		for _, entry := range logEntries {
			logEntryPrintHandler(entry, &TailOptions{
				Deployment: entry.Etag,
				Services:   []string{},
				Verbose:    false,
			}, mockTerm)
		}

		// Convert output to string array
		outputLines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")

		// Read expected output from file as plain text
		expectedLines := fileToStringArray(t, tc.outputfile)

		// Compare line by line
		t.Logf("Got %d output lines, expected %d lines for %s", len(outputLines), len(expectedLines), tc.outputfile)

		maxLines := max(len(outputLines), len(expectedLines))
		for i := range maxLines {
			var actualLine, expectedLine string

			if i < len(outputLines) {
				actualLine = strings.TrimSpace(outputLines[i])
			}
			if i < len(expectedLines) {
				expectedLine = strings.TrimSpace(expectedLines[i])
			}

			// Strip ANSI codes from actual output
			actualLineClean := term.StripAnsi(actualLine)
			expectedLineClean := term.StripAnsi(expectedLine)

			// Normalize whitespace: convert tabs to spaces for consistent comparison
			actualLineClean = strings.ReplaceAll(actualLineClean, "\t", "    ")
			expectedLineClean = strings.ReplaceAll(expectedLineClean, "\t", "    ")

			// Normalize \
			// actualLineClean = strings.ReplaceAll(actualLineClean, "\", "    ")
			expectedLineClean = strings.ReplaceAll(expectedLineClean, "\\\"", "\"")
			expectedLineClean = strings.ReplaceAll(expectedLineClean, "\t", "    ")
			if actualLineClean != expectedLineClean {
				t.Errorf("File %s Line %d mismatch:\nActual:   %q\nExpected: %q", tc.outputfile, i, actualLineClean, expectedLineClean)
			}
		}
	}
}

func fileToStringArray(t *testing.T, fileName string) []string {
	expectedFile, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("Failed to open expected output file: %v", err)
	}
	defer expectedFile.Close()

	var expectedLines []string
	scanner := bufio.NewScanner(expectedFile)
	for scanner.Scan() {
		expectedLines = append(expectedLines, scanner.Text())
	}
	return expectedLines
}

func jsonFileToLogEntry(t *testing.T, fileName string) []*defangv1.LogEntry {
	file, err := os.Open(fileName)
	if err != nil {
		t.Fatalf("Failed to open test data file: %v", err)
	}
	defer file.Close()

	var logEntries []*defangv1.LogEntry
	scanner := bufio.NewScanner(file)
	lineNum := 0

	// Read each line and unmarshal it
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		var logEntry defangv1.LogEntry
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			t.Fatalf("Failed to unmarshal line %d: %v\nLine content: %s", lineNum, err, line)
		}

		logEntries = append(logEntries, &logEntry)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Error while reading file: %v", err)
	}

	t.Logf("Successfully loaded %d log entries from %s", len(logEntries), fileName)
	return logEntries
}

func TestTailOptions_String(t *testing.T) {
	tests := []struct {
		name string
		to   TailOptions
		want string
	}{
		{
			name: "with deployment",
			to: TailOptions{
				Verbose:    true,
				Deployment: "deploy123",
			},
			want: " --deployment=deploy123",
		},
		{
			name: "with services",
			to: TailOptions{
				Verbose:  true,
				Services: []string{"svc1", "svc2"},
			},
			want: " svc1 svc2",
		},
		{
			name: "with services and follow",
			to: TailOptions{
				Verbose:  true,
				Services: []string{"svc1", "svc2"},
				Follow:   true,
			},
			want: " --follow svc1 svc2",
		},
		{
			name: "with since and until",
			to: TailOptions{
				Verbose: true,
				Since:   time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
				Until:   time.Date(2024, 1, 2, 4, 4, 5, 0, time.UTC),
			},
			want: " --since=2024-01-02T03:04:05Z --until=2024-01-02T04:04:05Z",
		},
		{
			name: "with since and follow",
			to: TailOptions{
				Verbose: true,
				Since:   time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
				Follow:  true,
			},
			want: " --since=2024-01-02T03:04:05Z --follow",
		},
		{
			name: "with until and follow",
			to: TailOptions{
				Verbose: true,
				Until:   time.Date(2024, 1, 2, 5, 4, 5, 0, time.UTC),
				Follow:  true,
			},
			want: " --until=2024-01-02T05:04:05Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.to.String(); got != tt.want {
				t.Errorf("TailOptions.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
