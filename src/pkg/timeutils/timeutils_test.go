package timeutils

import (
	"testing"
	"time"
)

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
		{"1767075448030", time.UnixMilli(1767075448030)},
		{"1767075448", time.Unix(1767075448, 0)},
		{"1767075448.03", time.Unix(1767075448, 30000000)},
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
