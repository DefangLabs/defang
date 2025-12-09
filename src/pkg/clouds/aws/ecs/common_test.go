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
