package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStackManagerForCommand(t *testing.T) {
	tests := []struct {
		name           string
		directory      string
		projectName    string
		expectedTarget string
		expectedError  string
	}{
		{
			name:           "inside a directory without a project",
			directory:      "without-project",
			expectedTarget: "",
			expectedError:  "no local stack files found; create a new stack or use --project-name to load known stacks",
		},
		{
			name:           "inside a project directory without a stack directory",
			directory:      "without-stack",
			expectedTarget: ".",
		},
		{
			name:           "inside a nested directory within a project without a stack directory",
			directory:      "without-stack/child",
			expectedTarget: "..",
		},
		{
			name:           "inside a project directory with a stack directory",
			directory:      "with-project-and-stack",
			expectedTarget: ".",
		},
		{
			name:           "inside a nested directory within a project with a stack directory",
			directory:      "with-project-and-stack/child",
			expectedTarget: "..",
		},
		{
			name:          "outside a project directory",
			directory:     ".",
			projectName:   "myproject",
			expectedError: "no local stack files found; create a new stack or use --project-name to load known stacks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			var err error
			if err != nil {
				t.Fatalf("failed to create temp directory: %v", err)
			}
			// copy testdata to tempDir
			err = copyDir("testdata", tempDir)
			if err != nil {
				t.Fatalf("failed to copy testdata: %v", err)
			}
			t.Cleanup(func() { os.RemoveAll(tempDir) })
			testDir := filepath.Join(tempDir, tt.directory)
			t.Chdir(testDir)

			loader := compose.NewLoader(compose.WithProjectName(tt.projectName))
			sm, err := newStackManagerForLoader(t.Context(), loader)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
				return
			}
			require.NoError(t, err, "expected no error but got one")

			absTestDirectory, err := filepath.Abs(".")
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}
			actualTarget := sm.TargetDirectory()
			relativeTarget, err := filepath.Rel(absTestDirectory, actualTarget)
			if err != nil {
				t.Fatalf("failed to get relative path: %v", err)
			}
			assert.Equal(t, tt.expectedTarget, relativeTarget)
		})
	}
}

func copyDir(src string, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := src + string(os.PathSeparator) + entry.Name()
		dstPath := dst + string(os.PathSeparator) + entry.Name()
		if entry.IsDir() {
			if err := os.Mkdir(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}
