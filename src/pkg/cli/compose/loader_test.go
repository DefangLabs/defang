package compose

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLoader(t *testing.T) {
	composeRegex := regexp.MustCompile(`^(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../../tests", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			loader := Loader{path}
			proj, err := loader.LoadCompose(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			yaml, err := proj.MarshalYAML()
			if err != nil {
				t.Fatal(err)
			}

			// Replace the absolute path in context to make the .golden file portable
			absPath, _ := filepath.Abs(path)
			yaml = bytes.ReplaceAll(yaml, []byte(filepath.Dir(absPath)), []byte{'.'})

			golden, err := os.ReadFile(path + ".golden")
			if err != nil {
				os.WriteFile(path+".golden", yaml, 0644)
			} else if string(yaml) != string(golden) {
				t.Errorf("Result mismatch, written as %s.mismatch", path)
				os.WriteFile(path+".mismatch", yaml, 0644)
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
