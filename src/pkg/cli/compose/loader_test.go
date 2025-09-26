package compose

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DefangLabs/defang/src/pkg"
)

func TestLoader(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
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

func testRunCompose(t *testing.T, f func(t *testing.T, path string)) {
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
