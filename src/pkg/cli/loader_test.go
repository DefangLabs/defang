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
			proj, err := loader.LoadCompose()
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
				t.Errorf("Result mismatch, written as %s.mismatch", path)
				os.WriteFile(path+".mismatch", bytes, 0644)
			}
		})
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestProjectNameSafe(t *testing.T) {
	tests := []struct {
		name, safe string
	}{
		{"", ""},
		{"abc123", "abc123"},
		{"abc-123_456", "abc-123_456"},
		{"01234", "01234"},
		{"01234-abc-789", "01234-abc-789"},
		{"a b", "a_b"},
		{"ABC123DeF", "abc123def"},
		{"_abc", "0_abc"},
		{"統一碼", "0___"},
		{"Party🎉", "party_"},
		{"👍Project!!!👍", "0_project____"},
		{"Fancy_Project-Name/With/Slashes", "fancy_project-name_with_slashes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ProjectNameSafe(tt.name); got != tt.safe {
				t.Errorf("ProjectNameSafe() = %v, want %v", got, tt.safe)
			}
		})
	}
}
