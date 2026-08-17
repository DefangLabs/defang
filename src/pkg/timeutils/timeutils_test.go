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
		{"7d", now.Add(-7 * 24 * time.Hour)},
		{"7d12h", now.Add(-7*24*time.Hour - 12*time.Hour)},
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

func TestParseDuration(t *testing.T) {
	tests := []struct {
		str     string
		want    time.Duration
		wantErr bool
	}{
		{str: "12h", want: 12 * time.Hour},
		{str: "1h30m", want: 90 * time.Minute},
		{str: "7d", want: 7 * 24 * time.Hour},
		{str: "7d12h", want: 7*24*time.Hour + 12*time.Hour},
		{str: " 7D12H ", want: 7*24*time.Hour + 12*time.Hour}, // trimmed and lowercased
		{str: "0d", want: 0},
		{str: "-1h", want: -time.Hour},                   // no days prefix: Go sign rules apply
		{str: "7d-1h", want: 7*24*time.Hour - time.Hour}, // negative remainder, positive total
		{str: "106751d", want: 106751 * 24 * time.Hour},  // max representable whole days
		{str: "", wantErr: true},
		{str: "7", wantErr: true},
		{str: "1.5d", wantErr: true},
		{str: "-1d", wantErr: true},
		{str: "1d-25h", wantErr: true},     // negative total with days prefix
		{str: "106752d", wantErr: true},    // days alone overflow int64
		{str: "106751d24h", wantErr: true}, // remainder pushes total past int64
		{str: "d12h", wantErr: true},
		{str: "7d1d", wantErr: true},
		{str: "7dd", wantErr: true},
		{str: "1w", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			got, err := ParseDuration(tt.str)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDuration(%q) error = %v, wantErr %v", tt.str, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.str, got, tt.want)
			}
		})
	}
}
