package cfn

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGetCacheRepoPrefix(t *testing.T) {
	// Test cases
	tests := []struct {
		prefix string
		suffix string
		want   string
	}{
		{
			prefix: "short-",
			suffix: "ecr-public",
			want:   "short-ecr-public",
		},
		{
			prefix: "short-",
			suffix: "docker-public",
			want:   "short-docker-public",
		},
		{
			prefix: "loooooooooong-",
			suffix: "docker-public",
			want:   "fab852-docker-public",
		},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := getCacheRepoPrefix(tt.prefix, tt.suffix); got != tt.want {
				t.Errorf("getCacheRepoPrefix() = %q, want %q", got, tt.want)
			} else if len(got) > maxCachePrefixLength {
				t.Errorf("getCacheRepoPrefix() = %q, want length <= %v", got, maxCachePrefixLength)
			}
		})
	}
}

func TestCreateTemplate(t *testing.T) {
	template, err := CreateTemplate("test", []types.Container{
		{Image: "alpine:latest"},
		{Image: "docker.io/library/alpine:latest"},
		{Image: "public.ecr.aws/docker/library/alpine:latest"},
	})
	if err != nil {
		t.Fatalf("Error creating template: %v", err)
	}
	actual, err := template.YAML()
	if err != nil {
		t.Fatalf("Error generating template YAML: %v", err)
	}
	const golden = "testdata/golden.yaml"
	expected, err := os.ReadFile(golden)
	if err != nil {
		if os.IsNotExist(err) {
			os.WriteFile(golden, actual, 0644)
			t.Fatalf("Golden file created: %s", golden)
		} else {
			t.Fatalf("Error reading golden file: %v", err)
		}
	}
	assert.Equal(t, string(expected), string(actual))
}
