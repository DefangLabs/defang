package term

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"
)

var tests = []struct {
	msg, output string
	newLine     bool
}{
	{"Hello, World!", "Hello, World!\n", true},
	{"Hello, World!\r", "Hello, World!\r", true},
	{"Hello, World!\n", "Hello, World!\n", true},
	{"", "", true},
	{"Hello, World!", "Hello, World!", false},
	{"Hello, World!\r", "Hello, World!\r", false},
	{"Hello, World!\n", "Hello, World!\n", false},
	{"", "", false},
}

func TestOutput(t *testing.T) {
	for _, test := range tests {
		var buf strings.Builder
		out := termenv.NewOutput(&buf)
		out.Profile = termenv.Ascii // Disable color
		if _, err := output(out, InfoColor, test.msg, test.newLine); err != nil {
			t.Errorf("output(out, InfoColor, %q) results in error: %v", test.msg, err)
		}
		if buf.String() != test.output {
			t.Errorf("output(out, InfoColor, %q) = %q, want %q", test.msg, buf.String(), test.output)
		}
	}
	// Output(Stdout, InfoColor, "Hello, World!")
}
