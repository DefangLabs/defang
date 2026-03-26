package codebuild

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

func TestBuildspec(t *testing.T) {
	tests := []struct {
		name       string
		workingDir string
		cmd        []string
		wantErr    bool
	}{
		{
			name:       "simple command",
			workingDir: "/app",
			cmd:        []string{"echo", "hello"},
		},
		{
			name:       "command with spaces in args",
			workingDir: "/app",
			cmd:        []string{"echo", "hello world"},
		},
		{
			name:       "custom working dir",
			workingDir: "/workspace/myproject",
			cmd:        []string{"node", "lib/index.js"},
		},
		{
			name:       "empty working dir",
			workingDir: "",
			cmd:        []string{"echo", "hello"},
			wantErr:    true,
		},
		{
			name:       "empty command",
			workingDir: "/app",
			cmd:        nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildspec(tt.workingDir, tt.cmd...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("buildspec() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			// Verify it's valid YAML by round-tripping through the struct
			var parsed buildspecDoc
			if err := yaml.Unmarshal([]byte(got), &parsed); err != nil {
				t.Fatalf("buildspec produced invalid YAML: %v\n%s", err, got)
			}

			if parsed.Version != "0.2" {
				t.Errorf("expected version 0.2, got %q", parsed.Version)
			}

			commands := parsed.Phases.Build.Commands
			if len(commands) != 1 {
				t.Fatalf("expected 1 command, got %d", len(commands))
			}

			// Should contain cd to working dir
			if !strings.Contains(commands[0], tt.workingDir) {
				t.Errorf("command %q does not contain working dir %q", commands[0], tt.workingDir)
			}
		})
	}
}
