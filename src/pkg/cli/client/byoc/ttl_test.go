package byoc

import (
	"testing"
	"time"
)

func TestParseTTL(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		value   string
		want    string
		wantErr bool
	}{
		// no self-destruct
		{value: "", want: ""},
		{value: "0", want: "0"},
		{value: "never", want: "never"},
		{value: "Never", want: "never"},
		{value: " never ", want: "never"},
		// plain Go durations pass through
		{value: "12h", want: "12h"},
		{value: "90m", want: "90m"},
		{value: "1h30m", want: "1h30m"},
		// whole-days prefix passes through (the CD parses the same syntax)
		{value: "7d", want: "7d"},
		{value: "7d12h", want: "7d12h"},
		{value: "1d1h30m", want: "1d1h30m"},
		// out-of-bounds durations pass the syntax check; the CD enforces bounds
		{value: "30m", want: "30m"},
		{value: "9999d", want: "9999d"},
		// timestamps translate to the duration from now until then
		{value: "2026-08-17T12:00:00Z", want: "24h0m0s"},
		{value: "2026-08-16T13:30:00Z", want: "1h30m0s"},
		{value: "1786968000", want: "24h0m0s"},         // unix seconds for 2026-08-17T12:00:00Z
		{value: "2026-08-16T11:00:00Z", wantErr: true}, // in the past
		{value: "2026-08-16T12:00:00Z", wantErr: true}, // now is not in the future
		{value: "12", wantErr: true},                   // unix seconds in 1970
		// rejected syntax
		{value: "abc", wantErr: true},
		{value: "1w", wantErr: true},
		{value: "1.5d", wantErr: true},
		{value: "-1d", wantErr: true},
		{value: "d12h", wantErr: true},
		{value: "7d1d", wantErr: true},
		{value: "7dd", wantErr: true},
		{value: "12h7d", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseTTL(tt.value, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTTL(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("ParseTTL(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestAddTTLEnv(t *testing.T) {
	tests := []struct {
		name    string
		ttl     string
		wantKey bool
	}{
		{name: "empty omits the key", ttl: "", wantKey: false},
		{name: "duration is set", ttl: "7d12h", wantKey: true},
		{name: "never is forwarded to cancel a scheduled self-destruct", ttl: "never", wantKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"OTHER": "x"}
			AddTTLEnv(env, tt.ttl)
			got, ok := env["DEFANG_TTL"]
			if ok != tt.wantKey {
				t.Fatalf("env[DEFANG_TTL] present = %v, want %v", ok, tt.wantKey)
			}
			if ok && got != tt.ttl {
				t.Errorf("env[DEFANG_TTL] = %q, want %q", got, tt.ttl)
			}
		})
	}
}
