//go:build integration

package docker

import (
	"context"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker test")
	}

	d := New()

	_, err := d.SetUp(context.Background(), []clouds.Container{{Image: "alpine:latest", Platform: d.platform}})
	if err != nil {
		t.Fatal(err)
	}
	defer d.TearDown(context.Background())

	id, err := d.Run(context.Background(), nil, "sh", "-c", "echo hello world")
	if err != nil {
		t.Fatal(err)
	}
	if id == nil || *id == "" {
		t.Fatal("id is empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = d.Tail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
}

func TestParsePlatform(t *testing.T) {
	tdt := []struct {
		platform string
		expected v1.Platform
	}{
		{
			platform: "linux/amd64",
			expected: v1.Platform{
				Architecture: "amd64",
				OS:           "linux",
			},
		},
		{
			platform: "linux/arm64/v8",
			expected: v1.Platform{
				Architecture: "arm64",
				OS:           "linux",
				Variant:      "v8",
			},
		},
		{
			platform: "linux/arm64",
			expected: v1.Platform{
				Architecture: "arm64",
				OS:           "linux",
			},
		},
		{
			platform: "arm64",
			expected: v1.Platform{
				Architecture: "arm64",
			},
		},
	}
	for _, tt := range tdt {
		t.Run(tt.platform, func(t *testing.T) {
			p := parsePlatform(tt.platform)
			if p.Architecture != tt.expected.Architecture {
				t.Errorf("expected architecture %q, got %q", tt.expected.Architecture, p.Architecture)
			}
			if p.OS != tt.expected.OS {
				t.Errorf("expected OS %q, got %q", tt.expected.OS, p.OS)
			}
			if p.Variant != tt.expected.Variant {
				t.Errorf("expected variant %q, got %q", tt.expected.Variant, p.Variant)
			}
		})
	}
}
