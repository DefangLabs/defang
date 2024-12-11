package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

func TestTail(t *testing.T) {
	var stdout, stderr bytes.Buffer
	testTerm := term.NewTerm(os.Stdin, &stdout, &stderr)
	testTerm.ForceColor(true)
	defaultTerm := term.DefaultTerm
	term.DefaultTerm = testTerm
	t.Cleanup(func() {
		term.DefaultTerm = defaultTerm
	})

	loader := compose.NewLoader(compose.WithPath("../../tests/testproj/compose.yaml"))
	proj, err := loader.LoadProject(context.Background())
	if err != nil {
		t.Fatalf("LoadProject() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := client.MockProvider{
		ServerStream: &client.MockServerStream{
			Resps: []*defangv1.TailResponse{
				{Service: "service1", Etag: "SOMEETAG", Host: "SOMEHOST", Entries: []*defangv1.LogEntry{
					{Message: "e1msg1", Timestamp: timestamppb.New(time.Now())},
					{Message: "e1msg2", Timestamp: timestamppb.New(time.Now()), Etag: "SOMEOTHERETAG"},                                              // Test event etag override the response etag
					{Message: "e1msg3", Timestamp: timestamppb.New(time.Now()), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST"},                      // override both etag and host
					{Message: "e1msg4", Timestamp: timestamppb.New(time.Now()), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST", Service: "service2"}, // override both etag, host and service
					{Message: "e1err1", Timestamp: timestamppb.New(time.Now()), Stderr: true},                                                       // Error message should be in stdout too when not raw
				}},
				{Service: "service1", Etag: "SOMEETAG", Host: "SOMEHOST", Entries: []*defangv1.LogEntry{ // Test entry etag does not affect the default values from response
					{Message: "e2err1", Timestamp: timestamppb.New(time.Now()), Stderr: true, Etag: "SOMEOTHERETAG"}, // Error message should be in stdout too when not raw
					{Message: "e2msg1", Timestamp: timestamppb.New(time.Now()), Etag: "ENTRIES2ETAG"},
					{Message: "e2msg2", Timestamp: timestamppb.New(time.Now())},
					{Message: "e2msg3", Timestamp: timestamppb.New(time.Now()), Etag: "SOMEOTHERETAG2", Host: "SOMEOTHERHOST", Service: "service2"}, // override both etag, host and service
					{Message: "e2msg4", Timestamp: timestamppb.New(time.Now())},
				}},
			},
		},
	}

	err = Tail(ctx, p, TailOptions{ProjectName: proj.Name, Verbose: true}) // Output host
	t.Log(err)

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
	}

	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")

	if len(got) != len(expectedLogs) {
		t.Fatalf("Expecting %v lines of log, but only got %v", len(expectedLogs), len(got))
	}

	for i, g := range got {
		e := expectedLogs[i]
		g = term.StripAnsi(g)
		if strings.SplitN(g, " ", 2)[1] != e { // Remove the date from the log entry
			t.Errorf("Tail() = %v, want %v", g, e)
		}
	}

	if stderr.Len() > 0 {
		t.Errorf("Tail() stderr = %v, want empty", stderr.String())
	}
}
