package term

import (
	"strconv"
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

func TestOutputf(t *testing.T) {
	tests := []struct {
		msg, output string
		profile     termenv.Profile
	}{
		{"Hello, World!", "Hello, World!\n", termenv.Ascii},
		{"Hello, World!\r", "Hello, World!\r", termenv.Ascii},
		{"Hello, World!\n", "Hello, World!\n", termenv.Ascii},
		{"", "\n", termenv.Ascii},
		{"Hello, World!", "\x1b[95mHello, World!\n\x1b[0m", termenv.ANSI},
		{"Hello, World!\r", "\x1b[95mHello, World!\r\x1b[0m", termenv.ANSI},
		{"Hello, World!\n", "\x1b[95mHello, World!\n\x1b[0m", termenv.ANSI},
		{"", "\x1b[95m\n\x1b[0m", termenv.ANSI},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var buf strings.Builder
			out := termenv.NewOutput(&buf)
			out.Profile = test.profile
			if _, err := outputf(out, InfoColor, test.msg); err != nil {
				t.Errorf("outputf(out, InfoColor, %q) results in error: %v", test.msg, err)
			}
			if buf.String() != test.output {
				t.Errorf("outputf(out, InfoColor, %q) = %q, want %q", test.msg, buf.String(), test.output)
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
