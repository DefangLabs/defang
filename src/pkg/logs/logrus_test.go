package logs

import "testing"

func TestIsKanikoError(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{"empty", "", false},
		{"not kaniko", "error building image: error building stage: failed to execute command: waiting for process to exit: exit status 1", true},
		{"info", "INFO[0001] Retrieving image manifest alpine:latest", false},
		{"trace", "TRAC[0001] blah", false},
		{"debug", "DEBU[0001] blah", false},
		{"warn", "WARN[0001] Failed to retrieve image library/alpine:latest", false},
		{"error", "ERRO[0001] some err", true},
		{"fatal", "FATA[0001] some err", true},
		{"panic", "PANI[0001] some err", true},
		{"trace long", "TRACE long trace message", false},
		{"ansi info", "\033[36mINFO\033[0m[0001] colored blue", false},
		{"ansi warn", "\033[33mWARN\033[0m[0001] colored yellow", false},
		{"ansi err", "\033[31mERRO\033[0m[0001] colored red", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLogrusError(tt.msg); got != tt.want {
				t.Errorf("isKanikoError() = %v, want %v", got, tt.want)
			}
		})
	}
}
