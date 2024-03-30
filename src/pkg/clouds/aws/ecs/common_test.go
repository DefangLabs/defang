package ecs

import (
	"testing"
)

func TestPlatformToArch(t *testing.T) {
	tests := []struct {
		platform string
		wantArch string
		wantOs   string
	}{
		{"", "", ""},
		{"blah", "BLAH", ""}, // invalid platform
		{"amd64", "X86_64", ""},
		{"arm64", "ARM64", ""},
		{"linux/amd64", "X86_64", "LINUX"},
		{"linux/arm64", "ARM64", "LINUX"},
		{"linux/arm64/v8", "ARM64", "LINUX"},
		{"linux/blah", "BLAH", "LINUX"},     // invalid platform
		{"windows/blah", "BLAH", "WINDOWS"}, // invalid platform
		{"windows/amd64", "X86_64", "WINDOWS"},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			arch, os := PlatformToArchOS(tt.platform)
			if os != tt.wantOs {
				t.Errorf("PlatformToArch() os = %q, want %q", os, tt.wantOs)
			}
			if arch != tt.wantArch {
				t.Errorf("PlatformToArch() arch = %q, want %q", arch, tt.wantArch)
			}
		})
	}
}

func TestGetAccountID(t *testing.T) {
	a := AwsEcs{
		TaskDefARN: "arn:aws:ecs:us-east-1:123456789012:task-definition/defang-ecs-2021-08-31-163042",
	}
	if got := a.getAccountID(); got != "123456789012" {
		t.Errorf("GetAccountID() = %v, want 123456789012", got)
	}
}
