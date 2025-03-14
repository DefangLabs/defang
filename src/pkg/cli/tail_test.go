package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
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

func TestParseTimeOrDuration(t *testing.T) {
	now := time.Now()
	tdt := []struct {
		td   string
		want time.Time
	}{
		{"", time.Time{}},
		{"1s", now.Add(-time.Second)},
		{"2m3s", now.Add(-2*time.Minute - 3*time.Second)},
		{"2024-01-01T00:00:00Z", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{"2024-02-01T00:00:00.500Z", time.Date(2024, 2, 1, 0, 0, 0, 5e8, time.UTC)},
		{"2024-03-01T00:00:00+07:00", time.Date(2024, 3, 1, 0, 0, 0, 0, time.FixedZone("", 7*60*60))},
		{"00:01:02.040", time.Date(now.Year(), now.Month(), now.Day(), 0, 1, 2, 4e7, now.Location())}, // this test will fail if it's run at midnight UTC :(
	}
	for _, tt := range tdt {
		t.Run(tt.td, func(t *testing.T) {
			got, err := ParseTimeOrDuration(tt.td, now)
			if err != nil {
				t.Errorf("ParseTimeOrDuration() error = %v", err)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseTimeOrDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

type mockTailProvider struct {
	client.Provider
	ServerStreams []client.ServerStream[defangv1.TailResponse]
	Reqs          []*defangv1.TailRequest
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

func (m *mockTailProvider) MockTimestamp(timestamp time.Time) *mockTailProvider {
	return &mockTailProvider{
		ServerStreams: []client.ServerStream[defangv1.TailResponse]{
			&client.MockServerStream{
				Resps: []*defangv1.TailResponse{
					{Entries: []*defangv1.LogEntry{
						{Timestamp: timestamppb.New(timestamp)},
					}},
				},
			}, &client.MockServerStream{Error: io.EOF},
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
			&client.MockServerStream{
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
			&client.MockServerStream{Error: io.EOF},
		},
	}

	err := Tail(context.Background(), p, projectName, TailOptions{Verbose: true}) // Output host
	if err != io.EOF {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	expectedLogs := []string{
		"SOMEETAG service1 SOMEHOST e1msg1",
		"SOMEOTHERETAG service1 SOMEHOST e1msg2",
		"SOMEOTHERETAG2 service1 SOMEOTHERHOST e1msg3",
		"SOMEOTHERETAG2 service2 SOMEOTHERHOST e1msg4",
		"SOMEETAG service1 SOMEHOST e1err1",
		"SOMEOTHERETAG service1 SOMEHOST e2err1",
		"ENTRIES2ETAG service1 SOMEHOST e2msg1",
		"SOMEETAG service1 SOMEHOST e2msg2",
		"SOMEOTHERETAG2 service2 SOMEOTHERHOST e2msg3",
		"SOMEETAG service1 SOMEHOST e2msg4",
		"! Reconnecting...\r                           \r",
	}

	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")

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
	if p.Reqs[0].LogType != 2 {
		t.Errorf("Tail() sent request with log type %v, want 2", p.Reqs[0].LogType)
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
	if p.Reqs[1].LogType != 2 {
		t.Errorf("Tail() sent request with log type %v, want 2", p.Reqs[0].LogType)
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

	defer cleanup()

	// Testing local time
	localTime := time.Now().Truncate(time.Second)

	// Create mock data for tail with local time
	const projectName = "project"
	localMock := &mockTailProvider{}
	localMock = localMock.MockTimestamp(localTime)

	// Start the terminal for local time test
	err := Tail(context.Background(), localMock, projectName, TailOptions{Verbose: true}) // Output host
	if err != nil {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	format := time.RFC3339Nano

	localTimeparse := strings.TrimSpace(term.StripAnsi(stdout.String()))
	convertedLocalTime, err := time.Parse(format, localTimeparse)
	if err != nil {
		t.Error("Error parsing time:", err)
	}

	if !convertedLocalTime.Equal(localTime) {
		t.Errorf("Original local time:%v != parse local time:%v", localTime, convertedLocalTime)
	}

	// Set "local" to "UTC"
	time.Local = time.UTC

	// Create the UTC time object
	utcTime := time.Now().Truncate(time.Second)

	// Setup terminal for UTC time test
	stdout2, stderr, cleanup2 := setupTestTerminal()
	if stderr.Len() > 0 {
		t.Errorf("Unexpected stderr output: %v", stderr.String())
	}

	defer cleanup2()

	// Create new mock data for tail with UTC time
	utcMock := &mockTailProvider{}
	utcMock = utcMock.MockTimestamp(utcTime)

	err = Tail(context.Background(), utcMock, projectName, TailOptions{Verbose: true})
	if err != nil {
		t.Errorf("Tail() error = %v, want io.EOF", err)
	}

	// Parse the time from the terminal for UTC time
	utcTimeParse := strings.TrimSpace(term.StripAnsi(stdout2.String()))
	convertedUTCTime, err := time.Parse(format, utcTimeParse)
	if err != nil {
		t.Error("Error parsing time:", err)
	}

	if !convertedUTCTime.Equal(utcTime) {
		t.Errorf("Original utc time:%v != parse utc time:%v", utcTime, convertedUTCTime)
	}
}
