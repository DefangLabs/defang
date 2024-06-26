package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLoader(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := Loader{path}
		proj, err := loader.LoadCompose(context.Background())
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
		return diff(string(actual), string(golden))
	}
}

func diff(actualRaw, goldenRaw string) error {
	linesActual := strings.Split(actualRaw, "\n")
	linesGolden := strings.Split(goldenRaw, "\n")
	for i, actual := range linesActual {
		if i >= len(linesGolden) {
			return fmt.Errorf("+ expected - actual\n+EOF\n-%s", actual)
		}
		if actual != linesGolden[i] {
			return fmt.Errorf("+ expected - actual\n+%s\n-%s", linesGolden[i], actual)
		}
	}
	if len(linesActual) < len(linesGolden) {
		return fmt.Errorf("+ expected - actual\n+%s\n-EOF", linesGolden[len(linesActual)])
	}
	return nil
}

func testRunCompose(t *testing.T, f func(t *testing.T, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`(?i)^(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../../tests", func(path string, d os.DirEntry, err error) error {
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
