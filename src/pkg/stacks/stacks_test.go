package stacks

import (
	"os"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/stretchr/testify/assert"
)

func TestCreate(t *testing.T) {
	tests := []struct {
		name             string
		parameters       StackParameters
		expectErr        bool
		expectedFilename string
	}{
		{
			name: "valid parameters",
			parameters: StackParameters{
				Name:     "teststack",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr:        false,
			expectedFilename: ".defangrc.teststack",
		},
		{
			name: "missing stack name",
			parameters: StackParameters{
				Name:     "",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr: true,
		},
		{
			name: "name with whitespaces",
			parameters: StackParameters{
				Name:     "invalid stack",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr: true,
		},
		{
			name: "single letter ok",
			parameters: StackParameters{
				Name: "a",
			},
			expectErr:        false,
			expectedFilename: ".defangrc.a",
		},
		{
			name: "hyphen not ok",
			parameters: StackParameters{
				Name: "invalid-name",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			filename, err := Create(tt.parameters)
			if (err != nil) != tt.expectErr {
				t.Errorf("Create() error = %v, expectErr %v", err, tt.expectErr)
			}

			// Cleanup created file if no error expected
			if !tt.expectErr {
				os.Remove(".defangrc." + tt.parameters.Name)
			}

			if filename != tt.expectedFilename {
				t.Errorf("Create() = %q, want %q", filename, tt.expectedFilename)
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Run("no stacks present", func(t *testing.T) {
		t.Chdir(t.TempDir())
		stacks, err := List()
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(stacks) != 0 {
			t.Errorf("Expected 0 stacks, got %d", len(stacks))
		}
	})

	t.Run("stacks present", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// Create dummy stack files
		os.Create(".defangrc.stack1")
		os.Create(".defangrc.stack2")

		stacks, err := List()
		if err != nil {
			t.Errorf("List() error = %v", err)
		}
		if len(stacks) != 2 {
			t.Errorf("Expected 2 stacks, got %d", len(stacks))
		}
	})
}

func TestRemove(t *testing.T) {
	t.Run("remove existing stack", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// Create dummy stack file
		stackFile := ".defangrc.stack_to_remove"
		os.Create(stackFile)

		err := Remove("stack_to_remove")
		if err != nil {
			t.Errorf("Remove() error = %v", err)
		}
		if _, err := os.Stat(stackFile); !os.IsNotExist(err) {
			t.Errorf("Expected stack file to be removed")
		}
	})

	t.Run("remove non-existing stack", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := Remove("non_existing_stack")
		// expect an error when trying to remove a non-existing stack
		assert.Error(t, err)
		assert.ErrorContains(t, err, "remove .defangrc.non_existing_stack: no such file or directory")
	})
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		name            string
		params          StackParameters
		expectedContent string
	}{
		{
			name: "GCP provider",
			params: StackParameters{
				Name:     "teststack",
				Provider: cliClient.ProviderGCP,
				Region:   "us-central1",
				Mode:     modes.ModeBalanced,
			},
			expectedContent: "DEFANG_MODE=\"balanced\"\nDEFANG_PROVIDER=\"gcp\"\nGCP_LOCATION=\"us-central1\"",
		},
		{
			name: "AWS provider",
			params: StackParameters{
				Name:     "awsstack",
				Provider: cliClient.ProviderAWS,
				Region:   "us-east-1",
				Mode:     modes.ModeAffordable,
			},
			expectedContent: "AWS_REGION=\"us-east-1\"\nDEFANG_MODE=\"affordable\"\nDEFANG_PROVIDER=\"aws\"",
		},
		{
			name: "Unspecified mode",
			params: StackParameters{
				Name:     "nomodestack",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-1",
				Mode:     modes.ModeUnspecified,
			},
			expectedContent: "AWS_REGION=\"us-west-1\"\nDEFANG_PROVIDER=\"aws\"",
		},
		{
			name: "Empty region",
			params: StackParameters{
				Name:     "noregionstack",
				Provider: cliClient.ProviderGCP,
				Region:   "",
				Mode:     modes.ModeAffordable,
			},
			expectedContent: "DEFANG_MODE=\"affordable\"\nDEFANG_PROVIDER=\"gcp\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := Marshal(tt.params)
			assert.NoError(t, err)
			if content != tt.expectedContent {
				t.Errorf("Marshal() = %q, want %q", content, tt.expectedContent)
			}
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedParams StackParameters
	}{
		{
			name: "GCP provider",
			content: `DEFANG_PROVIDER=gcp
GCP_LOCATION=us-central1
DEFANG_MODE=balanced
`,
			expectedParams: StackParameters{
				Provider: cliClient.ProviderGCP,
				Region:   "us-central1",
				Mode:     modes.ModeBalanced,
			},
		},
		{
			name: "AWS provider",
			content: `DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
DEFANG_MODE=affordable
`,
			expectedParams: StackParameters{
				Provider: cliClient.ProviderAWS,
				Region:   "us-east-1",
				Mode:     modes.ModeAffordable,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := Parse(tt.content)
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}
			if params.Provider != tt.expectedParams.Provider ||
				params.Region != tt.expectedParams.Region ||
				params.Mode != tt.expectedParams.Mode {
				t.Errorf("Parse() = %v, want %v", params, tt.expectedParams)
			}
		})
	}
}
