package codebuild

import (
	"strings"
	"testing"

	cbtypes "github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"go.yaml.in/yaml/v4"
)

func TestEnvironmentTypeForImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  cbtypes.EnvironmentType
	}{
		{
			name:  "arm64 public CD image with digest",
			image: "public.ecr.aws/defang-io/cd:public-cd-image-fb20b70e-arm64@sha256:6cdb3f11e548700a673098642ed25e3fb50ebf3b39d533364f75d207bf66ea9b",
			want:  cbtypes.EnvironmentTypeArmContainer,
		},
		{
			name:  "x86_64 private CD image with digest",
			image: "426819183542.dkr.ecr.us-west-2.amazonaws.com/cd:nodejs-cd-image-e87eefcf-x86_64@sha256:7efe4a59ed9d1c06f4bf396d76dbfb0f1d4784c289999bf88664fabe43409793",
			want:  cbtypes.EnvironmentTypeLinuxContainer,
		},
		{
			name:  "aarch64 spelling",
			image: "public.ecr.aws/defang-io/cd:some-tag-aarch64",
			want:  cbtypes.EnvironmentTypeArmContainer,
		},
		{
			name:  "unknown arch defaults to linux",
			image: "aws/codebuild/amazonlinux2-x86_64-standard:5.0",
			want:  cbtypes.EnvironmentTypeLinuxContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := environmentTypeForImage(tt.image); got != tt.want {
				t.Errorf("environmentTypeForImage(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

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
