package term

import (
	"bytes"
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
		msgs     []warning
		expected []warning
	}{
		{[]warning{{message: "", color: WarnColor}, {message: "", color: WarnColor}}, []warning{{message: " ! \n", color: WarnColor}}},
		{[]warning{{message: "A", color: WarnColor}, {message: "A", color: WarnColor}}, []warning{{message: " ! A\n", color: WarnColor}}},
		{[]warning{{message: "A", color: WarnColor}, {message: "B", color: WarnColor}}, []warning{{message: " ! A\n", color: WarnColor}, {message: " ! B\n", color: WarnColor}}},
		{[]warning{{message: "B", color: WarnColor}, {message: "C", color: WarnColor}, {message: "A", color: WarnColor}}, []warning{{message: " ! A\n", color: WarnColor}, {message: " ! B\n", color: WarnColor}, {message: " ! C\n", color: WarnColor}}},
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
					t.Errorf("Expected %s unique warnings, got: %s", expected.message, uniqueWarnings[j].message)
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

			bytesWritten, err := term.FlushWarnings()
			if (err != nil) != test.expectErr {
				t.Errorf("FlushWarnings() error = %v, expectErr %v", err, test.expectErr)
			}
			bytesInExpected := 0
			for _, msg := range test.expected {
				bytesInExpected += len(msg)
			}

			if bytesInExpected != bytesWritten {
				t.Errorf("FlushWarnings() expected %d byteWritten, got %d", bytesInExpected, bytesWritten)
			}

			if term.getAllWarnings() != nil {
				t.Errorf("after FlushWarnings() expected no warnings, got %v", term.getAllWarnings())
			}
		})
	}
}
