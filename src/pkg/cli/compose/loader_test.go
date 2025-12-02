package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
)

func TestLoader(t *testing.T) {
	testAllComposeFiles(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(t.Context())
		if err != nil {
			t.Fatal(err)
		}

		yaml, err := proj.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}

		// Compare the output with the golden file
		if err := pkg.Compare(yaml, path+".golden"); err != nil {
			t.Error(err)
		}
	})
}

func testAllComposeFiles(t *testing.T, f func(t *testing.T, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`^(?i)(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../../testdata", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			t.Helper()
			t.Log(path)
			f(t, path)
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestHasSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "no substitution",
			input:    "${var}",
			expected: false,
		},
		{
			name:     "substitution",
			input:    "${var-def}",
			expected: true,
		},
		{
			name:     "escaped substitution",
			input:    "$${var-def}",
			expected: false,
		},
		{
			name:     "escaped dollar and substitution",
			input:    "$${var+def}",
			expected: false,
		},
		// following test not supported yet
		// {
		// 	name:     "escaped dollar and escaped substitution",
		// 	input:    "$$${var?def}",
		// 	expected: true,
		// },
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if hasSubstitution(tt.input, "var") != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, !tt.expected)
			}
		})
	}
}

func TestComposeEnv(t *testing.T) {
	t.Setenv("COMPOSE_PROJECT_NAME", "env_project_name")
	t.Setenv("COMPOSE_PATH_SEPARATOR", "|")
	t.Setenv("COMPOSE_FILE", "../../../testdata/multiple/compose1.yaml|../../../testdata/multiple/compose2.yaml")
	t.Setenv("COMPOSE_DISABLE_ENV_FILE", "1")

	loader := NewLoader()
	p, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, "env_project_name", p.Name)
	assert.Len(t, p.Services, 2)
	assert.Equal(t, types.NewMappingWithEquals([]string{"A=${A}"}), p.Services["service1"].Environment)
}
