package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStackManager(t *testing.T) {
	tests := []struct {
		name           string
		directory      string
		paths          []string
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
		{
			name:           "outside a project directory - refer to compose file in child",
			directory:      ".",
			paths:          []string{"with-project-and-stack/compose.yaml"},
			expectedTarget: "with-project-and-stack",
		},
		{
			name:           "outside a project directory - refer to compose file in sibling",
			directory:      "without-stack",
			paths:          []string{"../with-project-and-stack/compose.yaml"},
			expectedTarget: "../with-project-and-stack",
		},
		{
			name:           "outside a project directory - refer to compose file in parent",
			directory:      "with-project-and-stack/child",
			paths:          []string{"../compose.yaml"},
			expectedTarget: "..",
		},
		{
			name:           "outside a project directory - refer to compose file in child",
			directory:      ".",
			paths:          []string{"without-stack/compose.yaml"},
			expectedTarget: "without-stack",
		},
		{
			name:           "outside a project directory - refer to compose file in sibling",
			directory:      "with-project-and-stack",
			paths:          []string{"../without-stack/compose.yaml"},
			expectedTarget: "../without-stack",
		},
		{
			name:           "outside a project directory - refer to compose file in parent",
			directory:      "without-stack/child",
			paths:          []string{"../compose.yaml"},
			expectedTarget: "..",
		},
		{
			name:          "invalid compose file - returns error",
			directory:     "invalid-compose",
			paths:         []string{"compose.yaml"},
			expectedError: "additional properties 'blah' not allowed",
		},
	}

	oldProvider := global.Stack.Provider
	t.Cleanup(func() {
		global.Stack.Provider = oldProvider
	})
	global.Stack.Provider = "defang" // avoids invoking gRPC for listing remote stacks
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			// copy testdata to tempDir
			err := os.CopyFS(tempDir, os.DirFS("testdata"))
			if err != nil {
				t.Fatalf("failed to copy testdata: %v", err)
			}

			testDir := filepath.Join(tempDir, tt.directory)
			t.Chdir(testDir)

			t.Run("newStackManagerForLoader", func(t *testing.T) {
				loader := compose.NewLoader(compose.WithProjectName(tt.projectName), compose.WithPath(tt.paths...))
				sm, err := newStackManagerForLoader(t.Context(), loader)
				if tt.expectedError != "" {
					assert.ErrorContains(t, err, tt.expectedError)
					return
				}
				require.NoError(t, err, "expected no error but got one")

				if tt.expectedTarget == "" {
					assert.Equal(t, "", sm.TargetDirectory())
				} else {
					actualTarget := sm.TargetDirectory()
					expectedAbs, err := filepath.Abs(tt.expectedTarget)
					require.NoError(t, err, "failed to get absolute path")
					assert.Equal(t, expectedAbs, actualTarget)
				}
			})

			t.Run("newCommandSessionWithOpts", func(t *testing.T) {
				cmd := &cobra.Command{}
				cmd.Flags().String("project-name", tt.projectName, "")
				cmd.Flags().StringArray("file", tt.paths, "")
				cmd.SetContext(t.Context())
				_, err = newCommandSessionWithOpts(cmd, commandSessionOpts{})
				if tt.expectedError != "" {
					assert.ErrorContains(t, err, tt.expectedError)
					return
				}
				require.NoError(t, err, "expected no error but got one")
			})
		})
	}
}
