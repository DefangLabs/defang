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
			name:           "inside a directory without a project - defaults to defang provider",
			directory:      "without-project",
			expectedTarget: "",
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
			name:           "outside a project directory - default to defang provider",
			directory:      ".",
			projectName:    "myproject",
			expectedTarget: "",
		},
	}

	for _, tt := range tests {
		tempDir := t.TempDir()
		// copy testdata to tempDir
		err := copyDir("testdata", tempDir)
		if err != nil {
			t.Fatalf("failed to copy testdata: %v", err)
		}
		t.Run(tt.name, func(t *testing.T) {
			testDir := filepath.Join(tempDir, tt.directory)
			t.Chdir(testDir)

			loader := compose.NewLoader(compose.WithProjectName(tt.projectName))
			sm, err := newStackManagerForLoader(t.Context(), loader)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
				return
			}
			require.NoError(t, err, "expected no error but got one")

			if tt.expectedTarget == "" {
				assert.Equal(t, "", sm.TargetDirectory(t.Context()))
			} else {
				actualTarget := sm.TargetDirectory(t.Context())
				expectedAbs, err := filepath.Abs(tt.expectedTarget)
				if err != nil {
					t.Fatalf("failed to get absolute path: %v", err)
				}
				assert.Equal(t, expectedAbs, actualTarget)
			}
		})
	}
}

func copyDir(src string, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
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
