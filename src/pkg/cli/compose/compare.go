package compose

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

func Compare(actual []byte, goldenFile string) error {
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
