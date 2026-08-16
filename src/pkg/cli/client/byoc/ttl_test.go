package byoc

import "testing"

func TestValidateTTL(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		// no self-destruct
		{value: ""},
		{value: "0"},
		{value: "never"},
		{value: "Never"},
		{value: " never "},
		// plain Go durations
		{value: "12h"},
		{value: "90m"},
		{value: "1h30m"},
		// whole-days prefix
		{value: "7d"},
		{value: "7d12h"},
		{value: "1d1h30m"},
		// out-of-bounds values pass the syntax check; the CD enforces bounds
		{value: "30m"},
		{value: "9999d"},
		// rejected
		{value: "abc", wantErr: true},
		{value: "12", wantErr: true},
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
			err := ValidateTTL(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTTL(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestAddTTLEnv(t *testing.T) {
	tests := []struct {
		name    string
		envTTL  string
		wantKey bool
	}{
		{name: "unset omits the key", envTTL: "", wantKey: false},
		{name: "set copies the value", envTTL: "7d12h", wantKey: true},
		{name: "never is forwarded to cancel a previous TTL", envTTL: "never", wantKey: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(TTLEnvVar, tt.envTTL)
			env := map[string]string{"OTHER": "x"}
			AddTTLEnv(env)
			got, ok := env[TTLEnvVar]
			if ok != tt.wantKey {
				t.Fatalf("env[%s] present = %v, want %v", TTLEnvVar, ok, tt.wantKey)
			}
			if ok && got != tt.envTTL {
				t.Errorf("env[%s] = %q, want %q", TTLEnvVar, got, tt.envTTL)
			}
		})
	}
}
