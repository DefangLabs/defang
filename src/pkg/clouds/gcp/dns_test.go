package gcp

import (
	"regexp"
	"strings"
	"testing"
)

func TestSafeZoneName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, output string)
	}{
		{
			name:  "lowercases input",
			input: "MyZoneNAME",
			validate: func(t *testing.T, output string) {
				if output != "myzonename" {
					t.Fatalf("expected myzonename, got %q", output)
				}
			},
		},
		{
			name:  "replaces invalid characters with hyphens",
			input: "zone@name!with#chars",
			validate: func(t *testing.T, output string) {
				matched, _ := regexp.MatchString(`^[a-z0-9-]+$`, output)
				if !matched {
					t.Fatalf("output contains invalid characters: %q", output)
				}
			},
		},
		{
			name:  "trims leading and trailing hyphens",
			input: "---zone-name---",
			validate: func(t *testing.T, output string) {
				if output != "zone-name" {
					t.Fatalf("expected zone-name, got %q", output)
				}
			},
		},
		{
			name:  "prefix with letter if name starts with non-letter",
			input: "123-zone",
			validate: func(t *testing.T, output string) {
				if output[0] < 'a' || output[0] > 'z' {
					t.Fatalf("expected output to start with lower case letters, got %q", output)
				}
			},
		},
		{
			name:  "handles input that becomes empty",
			input: "!!!",
			validate: func(t *testing.T, output string) {
				if output == "" {
					t.Fatalf("expected non-empty zone name, got %q", output)
				}
			},
		},
		{
			name:  "does not exceed 63 characters",
			input: "a" + strings.Repeat("-b", 100),
			validate: func(t *testing.T, output string) {
				if len(output) > 63 {
					t.Fatalf("expected length <= 63, got %d", len(output))
				}
			},
		},
		{
			name:  "ends with lowercase letter or number",
			input: "zone-name-",
			validate: func(t *testing.T, output string) {
				last := output[len(output)-1]
				if !(last >= 'a' && last <= 'z') && !(last >= '0' && last <= '9') {
					t.Fatalf("invalid ending character: %q", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := SafeZoneName(tt.input)
			tt.validate(t, out)
		})
	}
}
