package cli

import "testing"

func TestIsProgressDot(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"empty", "", true},
		{"dot", ".", true},
		{"empty line", "\n", true},
		{"ansi dot", "\x1b[1m.\x1b[0m", true},
		{"pulumi dot", "\033[38;5;3m.\033[0m", true},
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
