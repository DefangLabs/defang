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

	linediff "github.com/andreyvit/diff"
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

	if actualRaw == goldenRaw {
		return nil
	}

	var buf strings.Builder
	const contextSize = 3

	diffs := linediff.LineDiffAsLines(goldenRaw, actualRaw)
	show := make([]bool, len(diffs))

	for i, diff := range diffs {
		if diff[0] == ' ' {
			continue
		}
		for j := i - contextSize; j < i+contextSize; j++ {
			if j >= 0 && j < len(diffs) {
				show[j] = true
			}
		}
	}

	w := len(fmt.Sprint(len(diffs)))
	for i, s := range show {
		if s {
			fmt.Fprintf(&buf, "%*v: %s\n", w, i, diffs[i])
		}
	}
	return fmt.Errorf("diff:\n%s", buf.String())
}

func testRunCompose(t *testing.T, f func(t *testing.T, path string)) {
	t.Helper()

	composeRegex := regexp.MustCompile(`^(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../../tests", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			f(t, path)
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
