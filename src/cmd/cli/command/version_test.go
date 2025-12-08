package command

import (
	"fmt"
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		cli    string
		latest string
		want   bool
	}{
		{"1.0.0", "v1.0.0", false},
		{"1.0.0", "v1.0.1", true},
		{"1.0.1", "v1.0.0", false},
		{"1.0.0", "v1.1.0", true},
		{"development", "v1.0.0", false},
		{"1234abc", "v1.0.0", false},
		{"1234567", "v1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v<>%v", tt.cli, tt.latest), func(t *testing.T) {
			if got := isNewer(tt.cli, tt.latest); got != tt.want {
				t.Errorf("IsNewer() = %v; want %v", got, tt.want)
			}
		})
	}
}

func TestGetCurrentVersion(t *testing.T) {
	RootCmd.Version = "development"
	dev := GetCurrentVersion()
	if dev != "development" {
		t.Errorf("GetCurrentVersion() = %v; want development", dev)
	}

	RootCmd.Version = "1.0.0" // as set by GoReleaser
	v := GetCurrentVersion()
	if v != "v1.0.0" {
		t.Errorf("GetCurrentVersion() = %v; want v1.0.0", v)
	}

	RootCmd.Version = "1234567" // GIT ref
	ref := GetCurrentVersion()
	if ref != "1234567" {
		t.Errorf("GetCurrentVersion() = %v; want 1234567", ref)
	}
}
