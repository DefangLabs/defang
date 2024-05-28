package cli

import (
	"testing"
	"time"
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
		{"01:02:03.040", time.Date(now.Year(), now.Month(), now.Day(), 1, 2, 3, 4e7, now.Location())},
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
