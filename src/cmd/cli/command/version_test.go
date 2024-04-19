package command

import (
	"context"
	"testing"
)

func TestGetCurrentVersion(t *testing.T) {
	dev := GetCurrentVersion()
	if dev != "development" {
		t.Errorf("GetCurrentVersion() = %v; want development", dev)
	}
	version = "1.0.0"
	v := GetCurrentVersion()
	if v != "v1.0.0" {
		t.Errorf("GetCurrentVersion() = %v; want v1.0.0", v)
	}
}

func TestGetLatestVersion(t *testing.T) {
	ctx := context.Background()
	v, err := GetLatestVersion(ctx)
	if err != nil {
		t.Fatalf("GetLatestVersion() error = %v; want nil", err)
	}
	if v == "" {
		t.Fatalf("GetLatestVersion() = %v; want non-empty", v)
	}
	if v[0] != 'v' {
		t.Errorf("GetLatestVersion() = %v; want v*", v)
	}
}
