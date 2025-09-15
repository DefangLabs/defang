package term

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

func TestOutput(t *testing.T) {
	tests := []struct {
		msg, output string
		profile     termenv.Profile
	}{
		{"Hello, World!", "Hello, World!", termenv.Ascii},
		{"Hello, World!\r", "Hello, World!\r", termenv.Ascii},
		{"Hello, World!\n", "Hello, World!\n", termenv.Ascii},
		{"", "", termenv.Ascii},
		{"Hello, World!", "\x1b[95mHello, World!\x1b[0m", termenv.ANSI},
		{"Hello, World!\r", "\x1b[95mHello, World!\r\x1b[0m", termenv.ANSI},
		{"Hello, World!\n", "\x1b[95mHello, World!\n\x1b[0m", termenv.ANSI},
		{"", "", termenv.ANSI},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var buf strings.Builder
			out := termenv.NewOutput(&buf)
			out.Profile = test.profile
			if _, err := output(out, InfoColor, test.msg); err != nil {
				t.Errorf("output(out, InfoColor, %q) results in error: %v", test.msg, err)
			}
			if buf.String() != test.output {
				t.Errorf("output(out, InfoColor, %q) = %q, want %q", test.msg, buf.String(), test.output)
			}
		})
	}
}

func TestEnableANSI(t *testing.T) {
	restore := EnableANSI()
	restore()
}

func TestStripAnsi(t *testing.T) {
	tests := []struct {
		msg, stripped string
	}{
		{"", ""},
		{"Hello, World!", "Hello, World!"},
		{"\x1bJHello, World!", "Hello, World!"},
		{"\x1bJHello, World!", "Hello, World!"},
		{"\x1b]0;Set console title!\x07", ""},
		{"\x1b[95mHello, World!\n\x1b[0m", "Hello, World!\n"},
		{"\x1b[95;1mHello, World!\r\x1b[0m", "Hello, World!\r"},
		{"\x1b[95mHello, World!\r", "Hello, World!\r"},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			if got := StripAnsi(test.msg); got != test.stripped {
				t.Errorf("StripAnsi(%q) = %q, want %q", test.msg, got, test.stripped)
			}
		})
	}
}

func TestAddingPrefix(t *testing.T) {
	defaultTerm := DefaultTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})
	var stdout, stderr bytes.Buffer
	DefaultTerm = NewTerm(os.Stdin, &stdout, &stderr)
	DefaultTerm.SetDebug(true)

	Debug("Hello, World!")
	Debugf("Hello, %s!", "World")
	Debug(" - Hello, World!")
	Debugf(" - Hello, %s!", "World")

	Info("Hello, World!")
	Infof("Hello, %s!", "World")
	Info(" * Hello, World!")
	Infof(" * Hello, %s!", "World")

	Warn("Hello, World!")
	Warnf("Hello, %s!", "World")
	Warn(" ! Hello, World!")
	Warnf(" ! Hello, %s!", "World")

	expected := []string{
		" - Hello, World!",
		" - Hello, World!",
		" - Hello, World!",
		" - Hello, World!",
		" * Hello, World!",
		" * Hello, World!",
		" * Hello, World!",
		" * Hello, World!",
		" ! Hello, World!",
		" ! Hello, World!",
		" ! Hello, World!",
		" ! Hello, World!",
	}
	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	for i, line := range got {
		if line != expected[i] {
			t.Errorf("Expected line %v in stdout to be %q, got %q", i, expected[i], line)
		}
	}

	if stderr.String() != "" {
		t.Errorf("Expected stderr to be empty, got %q", stderr.String())
	}
}

func TestInfoAddSpaceBetweenStrings(t *testing.T) {
	defaultTerm := DefaultTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})
	var stdout, stderr bytes.Buffer
	DefaultTerm = NewTerm(os.Stdin, &stdout, &stderr)
	DefaultTerm.SetDebug(true)

	Info("Hello", "World!")
	Info("Hello", 1, "World!")
	Info("Hello", errors.New("SomeErr"), "World!")
	domain := "api.domain.com"
	Printf("TLS cert for %v is ready", domain)

	expected := []string{
		" * Hello World!",
		" * Hello 1 World!",
		" * Hello SomeErr World!",
		"TLS cert for api.domain.com is ready",
	}
	got := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	for i, line := range got {
		if line != expected[i] {
			t.Errorf("Expected line %v in stdout to be %q, got %q", i, expected[i], line)
		}
	}

	if stderr.String() != "" {
		t.Errorf("Expected stderr to be empty, got %q", stderr.String())
	}
}

func TestIsTerminal(t *testing.T) {
	if IsTerminal() {
		t.Error("Expected IsTerminal() to return false")
	}
}
func TestWarn(t *testing.T) {
	tests := []struct {
		msgs     []string
		expected []string
	}{
		{[]string{"", ""}, []string{" ! \n"}},
		{[]string{"A", "A"}, []string{" ! A\n"}},
		{[]string{"A", "B"}, []string{" ! A\n", " ! B\n"}},
		{[]string{"B", "C", "A"}, []string{" ! A\n", " ! B\n", " ! C\n"}},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			defaultTerm := NewTerm(os.Stdin, os.Stdout, os.Stderr)
			for _, msg := range test.msgs {
				defaultTerm.Warn(msg)
			}

			uniqueWarnings := defaultTerm.getAllWarnings()
			if len(uniqueWarnings) != len(test.expected) {
				t.Errorf("Expected %d unique warnings, got %d", len(test.expected), len(uniqueWarnings))
			}
			for j, expected := range test.expected {
				if uniqueWarnings[j] != expected {
					t.Errorf("Expected %s unique warnings, got %s", expected, uniqueWarnings[j])
				}
			}
		})
	}
}
func TestFlushWarnings(t *testing.T) {
	tests := []struct {
		warnings  []string
		expected  []string
		expectErr bool
	}{
		{
			warnings:  []string{},
			expected:  []string{},
			expectErr: false,
		},
		{
			warnings:  []string{"Warning 1"},
			expected:  []string{" ! Warning 1\n"},
			expectErr: false,
		},
		{
			warnings:  []string{"Warning 1", "Warning 2"},
			expected:  []string{" ! Warning 1\n", " ! Warning 2\n"},
			expectErr: false,
		},
		{
			warnings:  []string{"Warning 2", "Warning 1", "Warning 1"},
			expected:  []string{" ! Warning 1\n", " ! Warning 2\n"},
			expectErr: false,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			term := NewTerm(os.Stdin, &stdout, &stderr)

			for _, warning := range test.warnings {
				term.Warn(warning)
			}

			_, err := term.FlushWarnings()
			if (err != nil) != test.expectErr {
				t.Errorf("FlushWarnings() error = %v, expectErr %v", err, test.expectErr)
			}
		})
	}
}

func TestJSONOutput(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		input    []any
		expected LogEntry
	}{
		{
			name:     "simple string",
			level:    "info",
			input:    []any{"Hello, World!"},
			expected: LogEntry{Level: "info", Message: "Hello, World!"},
		},
		{
			name:     "multiple values",
			level:    "warn",
			input:    []any{"Hello", "World", 123},
			expected: LogEntry{Level: "warn", Message: "Hello World 123"},
		},
		{
			name:     "empty input",
			level:    "debug",
			input:    []any{""},
			expected: LogEntry{Level: "debug", Message: ""},
		},
		{
			name:     "error level",
			level:    "error",
			input:    []any{"Something went wrong"},
			expected: LogEntry{Level: "error", Message: "Something went wrong"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			term := NewTerm(os.Stdin, &buf, &buf)
			out := termenv.NewOutput(&buf)

			_, err := term.outputJSON(test.level, out, test.input...)
			if err != nil {
				t.Errorf("outputJSON() error = %v", err)
			}

			// Parse the JSON output
			var entry LogEntry
			if err := json.Unmarshal(buf.Bytes()[:buf.Len()-1], &entry); err != nil { // -1 to remove trailing newline
				t.Errorf("Failed to unmarshal JSON: %v", err)
			}

			if entry.Level != test.expected.Level {
				t.Errorf("Expected level %q, got %q", test.expected.Level, entry.Level)
			}
			if entry.Message != test.expected.Message {
				t.Errorf("Expected message %q, got %q", test.expected.Message, entry.Message)
			}
		})
	}
}

func TestJSONMode(t *testing.T) {
	defaultTerm := DefaultTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	DefaultTerm = NewTerm(os.Stdin, &stdout, &stderr)
	DefaultTerm.SetJSONMode(true)
	DefaultTerm.SetDebug(true)

	// Test different log levels in JSON mode
	Debug("Debug message")
	Info("Info message")
	Warn("Warning message")
	Error("Error message")

	// Parse JSON outputs
	stdoutLines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	stderrLines := strings.Split(strings.TrimSpace(stderr.String()), "\n")

	// Check debug, info, warn go to stdout
	if len(stdoutLines) != 3 {
		t.Errorf("Expected 3 lines in stdout, got %d", len(stdoutLines))
	}

	// Check error goes to stderr
	if len(stderrLines) != 1 {
		t.Errorf("Expected 1 line in stderr, got %d", len(stderrLines))
	}

	// Verify JSON structure for each log level
	expectedLevels := []string{"debug", "info", "warn"}
	for i, line := range stdoutLines {
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Failed to unmarshal stdout line %d: %v", i, err)
		}
		if entry.Level != expectedLevels[i] {
			t.Errorf("Expected level %q, got %q", expectedLevels[i], entry.Level)
		}
	}

	// Verify error JSON
	var errorEntry LogEntry
	if err := json.Unmarshal([]byte(stderrLines[0]), &errorEntry); err != nil {
		t.Errorf("Failed to unmarshal stderr: %v", err)
	}
	if errorEntry.Level != "error" {
		t.Errorf("Expected error level, got %q", errorEntry.Level)
	}
}

func TestJSONModeDisabled(t *testing.T) {
	defaultTerm := DefaultTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	DefaultTerm = NewTerm(os.Stdin, &stdout, &stderr)
	DefaultTerm.SetDebug(true)

	// First enable JSON mode and test it works
	DefaultTerm.SetJSONMode(true)
	Info("JSON test message")

	// Check that JSON mode is working
	jsonOutput := stdout.String()
	var entry LogEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOutput)), &entry); err != nil {
		t.Errorf("Expected valid JSON when JSON mode is enabled, but got error: %v", err)
	}
	if entry.Level != "info" || entry.Message != "JSON test message" {
		t.Errorf("Expected JSON entry with level 'info' and message 'JSON test message', got: %+v", entry)
	}

	// Clear the buffer and disable JSON mode
	stdout.Reset()
	stderr.Reset()
	DefaultTerm.SetJSONMode(false) // Explicitly disable JSON mode

	Info("Test message")
	Warn("Warning message")

	// Should output regular colored text with prefixes
	output := stdout.String()
	if !strings.Contains(output, " * Test message") {
		t.Errorf("Expected info prefix ' * ', got: %q", output)
	}
	if !strings.Contains(output, " ! Warning message") {
		t.Errorf("Expected warn prefix ' ! ', got: %q", output)
	}

	// Should not be valid JSON
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		var entry LogEntry
		if json.Unmarshal([]byte(line), &entry) == nil {
			t.Errorf("Output should not be valid JSON when JSON mode is disabled, but got: %q", line)
		}
	}
}
