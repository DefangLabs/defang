package term

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

var tests = []struct {
	msg, output string
}{
	{"Hello, World!", "Hello, World!\n"},
	{"Hello, World!\r", "Hello, World!\r"},
	{"Hello, World!\n", "Hello, World!\n"},
	{"", ""},
}

func TestOutput(t *testing.T) {
	for _, test := range tests {
		var buf strings.Builder
		out := termenv.NewOutput(&buf)
		out.Profile = termenv.Ascii // Disable color
		if _, err := output(out, InfoColor, test.msg); err != nil {
			t.Errorf("output(out, InfoColor, %q) results in error: %v", test.msg, err)
		}
		if buf.String() != test.output {
			t.Errorf("output(out, InfoColor, %q) = %q, want %q", test.msg, buf.String(), test.output)
		}
	}
	// Output(Stdout, InfoColor, "Hello, World!")
}
