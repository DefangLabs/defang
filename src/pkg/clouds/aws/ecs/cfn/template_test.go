package cfn

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
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

var testContainers = []clouds.Container{
	{
		Image: "alpine:latest",
	},
	{
		Image: "docker.io/library/alpine:latest",
		Name:  "main2",
	},
	{
		Name:     "main3",
		Image:    "public.ecr.aws/docker/library/alpine:latest",
		Memory:   512_000_000,
		Platform: "linux/amd64",
	},
}

func createTestTemplate(t *testing.T) []byte {
	t.Helper()
	template, err := CreateTemplate("test", testContainers)
	if err != nil {
		t.Fatalf("Error creating template: %v", err)
	}
	templateBody, err := template.YAML()
	if err != nil {
		t.Fatalf("Error generating template YAML: %v", err)
	}
	return templateBody
}

func TestCreateTemplate(t *testing.T) {
	actual := createTestTemplate(t)

	const goldenYaml = "testdata/template.yaml"
	expected, err := os.ReadFile(goldenYaml)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.WriteFile(goldenYaml, actual, 0644)
			t.Fatalf("Golden file created: %s: %v", goldenYaml, err)
		} else {
			t.Fatalf("Error reading golden file: %v", err)
		}
	}

	// HACK: Unmarshal and marshal again to normalize indentation and formatting
	// Caused by https://github.com/aws/aws-toolkit-vscode/issues/8356
	var goldenObj interface{}
	err = yaml.Unmarshal(expected, &goldenObj)
	if err != nil {
		t.Fatalf("Error unmarshaling expected YAML: %v", err)
	}
	goldenBytes, err := yaml.Marshal(goldenObj)
	if err != nil {
		t.Fatalf("Error marshaling expected YAML: %v", err)
	}

	assert.Equal(t, string(goldenBytes), string(actual), "Generated template does not match golden file")
}
