package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

func TestLoader(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		proj, _, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		yaml, err := proj.MarshalYAML()
		if err != nil {
			t.Fatal(err)
		}

		// Compare the output with the golden file
		if err := compare(yaml, path+".golden"); err != nil {
			t.Error(err)
		}
	})
}

func compare(actual []byte, goldenFile string) error {
	// Replace the absolute path in context to make the golden file portable
	absPath, _ := filepath.Abs(goldenFile)
	actual = bytes.ReplaceAll(actual, []byte(filepath.Dir(absPath)), []byte{'.'})

	golden, err := os.ReadFile(goldenFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Failed to read golden file: %w", err)
		}
		return os.WriteFile(goldenFile, actual, 0644)
	} else {
		if err := diff(string(actual), string(golden)); err != nil {
			return fmt.Errorf("%s %w", goldenFile, err)
		}
	}
	return nil
}

func diff(actualRaw, goldenRaw string) error {
	if actualRaw == goldenRaw {
		return nil
	}

	edits := myers.ComputeEdits(span.URIFromPath("expected"), goldenRaw, actualRaw)
	diff := fmt.Sprint(gotextdiff.ToUnified("expected", "actual", goldenRaw, edits))
	return fmt.Errorf("mismatch:\n%s", diff)
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
