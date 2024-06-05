package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLoader(t *testing.T) {
	composeRegex := regexp.MustCompile(`^(docker-)?compose.ya?ml$`)
	err := filepath.WalkDir("../../tests", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !composeRegex.MatchString(d.Name()) {
			return err
		}

		t.Run(path, func(t *testing.T) {
			loader := ComposeLoader{path}
			proj, err := loader.LoadWithProjectName("test")
			if err != nil {
				t.Fatal(err)
			}
			bytes, err := proj.MarshalYAML()
			if err != nil {
				t.Fatal(err)
			}
			golden, err := os.ReadFile(path + ".golden")
			if err != nil {
				os.WriteFile(path+".golden", bytes, 0644)
			} else if string(bytes) != string(golden) {
				t.Error("Mismatch")
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}
