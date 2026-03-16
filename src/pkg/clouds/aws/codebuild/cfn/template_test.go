package cfn

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"
)

func createTestTemplate(t *testing.T) []byte {
	t.Helper()
	template, err := CreateTemplate("test")
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
	if os.IsNotExist(err) || os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenYaml, actual, 0644); err != nil {
			t.Fatalf("Error writing golden file: %v", err)
		}
		t.Fatalf("Golden file updated: %s", goldenYaml)
	} else if err != nil {
		t.Fatalf("Error reading golden file: %v", err)
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
