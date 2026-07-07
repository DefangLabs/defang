package stacks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/stretchr/testify/assert"
)

func TestMakeDefaultName(t *testing.T) {
	tests := []struct {
		provider client.ProviderID
		region   string
		expected string
	}{
		{client.ProviderAWS, "us-west-2", "awsuswest2"},
		{client.ProviderGCP, "us-central1", "gcpuscentral1"},
		{client.ProviderDO, "NYC3", "digitaloceannyc3"},
	}

	for _, tt := range tests {
		t.Run(tt.provider.String()+"_"+tt.region, func(t *testing.T) {
			result := MakeDefaultName(tt.provider, tt.region)
			if result != tt.expected {
				t.Errorf("MakeDefaultName() = %q, want %q", result, tt.expected)
			}
			if !stackNamePattern.MatchString(result) {
				t.Errorf("MakeDefaultName() produced invalid stack name: %q", result)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name             string
		parameters       Parameters
		expectErr        bool
		expectedFilename string
	}{
		{
			name: "valid parameters",
			parameters: Parameters{
				Name:     "teststack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
			expectErr:        false,
			expectedFilename: ".defang/teststack",
		},
		{
			name: "missing stack name",
			parameters: Parameters{
				Name:     "",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
			expectErr: true,
		},
		{
			name: "name with whitespaces",
			parameters: Parameters{
				Name:     "invalid stack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
			expectErr: true,
		},
		{
			name: "single letter ok",
			parameters: Parameters{
				Name:     "a",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
			expectErr:        false,
			expectedFilename: ".defang/a",
		},
		{
			name: "hyphen not ok",
			parameters: Parameters{
				Name:     "invalid-name",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			filename, err := CreateInDirectory(".", tt.parameters)
			if (err != nil) != tt.expectErr {
				t.Errorf("CreateInDirectory() error = %v, expectErr %v", err, tt.expectErr)
			}

			// Cleanup created file if no error expected
			if !tt.expectErr {
				if err := os.Remove(filename); err != nil {
					t.Errorf("Cleanup Remove() error = %v", err)
				}
			}

			if filename != tt.expectedFilename {
				t.Errorf("CreateInDirectory() = %q, want %q", filename, tt.expectedFilename)
			}
		})
	}
}

func TestRepeatCreate(t *testing.T) {
	t.Chdir(t.TempDir())
	params := Parameters{
		Name:     "repeattest",
		Provider: client.ProviderGCP,
		Region:   "us-central1",
		Recipe:   modes.RecipeBalanced,
	}

	_, err := CreateInDirectory(".", params)
	if err != nil {
		t.Errorf("First CreateInDirectory() error = %v", err)
	}

	_, err = CreateInDirectory(".", params)
	if err == nil {
		t.Errorf("Expected error on duplicate CreateInDirectory(), got nil")
	} else {
		assert.ErrorContains(t, err, "stack file already exists for \"repeattest\".")
		assert.ErrorContains(t, err, "If you want to overwrite it, please spin down the stack and remove stack file first.")
		assert.ErrorContains(t, err, "defang down --stack repeattest && rm .defang/repeattest")
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
		// Create dummy stack files with valid content
		os.Mkdir(Directory, 0700)
		stack1Path := filepath.Join(Directory, "stack1")
		stack2Path := filepath.Join(Directory, "stack2")
		os.WriteFile(stack1Path, []byte("DEFANG_PROVIDER=aws\nAWS_REGION=us-west-2\nDEFANG_MODE=affordable\n"), 0600)
		os.WriteFile(stack2Path, []byte("DEFANG_PROVIDER=gcp\nGOOGLE_REGION=us-central1\nDEFANG_MODE=balanced\n"), 0600)

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
		// Create dummy stack file with valid provider and region
		stackName := "stacktoremove"
		params := Parameters{
			Name:     stackName,
			Provider: client.ProviderAWS,
			Region:   "us-west-2",
			Recipe:   modes.RecipeAffordable,
		}
		stackFile, err := CreateInDirectory(".", params)
		if err != nil {
			t.Errorf("Setup CreateInDirectory() error = %v", err)
		}

		err = RemoveInDirectory(".", stackName)
		if err != nil {
			t.Errorf("RemoveInDirectory() error = %v", err)
		}
		if _, err := os.Stat(stackFile); !os.IsNotExist(err) {
			t.Errorf("Expected stack file to be removed")
		}
	})

	t.Run("remove non-existing stack", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := RemoveInDirectory(".", "nonexistingstack")
		// expect a not-found error when trying to remove a non-existing stack
		assert.True(t, errors.Is(err, os.ErrNotExist))
	})
}

func TestMarshal(t *testing.T) {
	tests := []struct {
		name            string
		params          Parameters
		expectedContent string
	}{
		{
			name: "GCP provider",
			params: Parameters{
				Name:     "teststack",
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Recipe:   modes.RecipeBalanced,
			},
			expectedContent: "DEFANG_PROVIDER=\"gcp\"\nDEFANG_RECIPE=\"balanced\"\nGOOGLE_REGION=\"us-central1\"",
		},
		{
			name: "AWS provider",
			params: Parameters{
				Name:     "awsstack",
				Provider: client.ProviderAWS,
				Region:   "us-east-1",
				Recipe:   modes.RecipeAffordable,
			},
			expectedContent: "AWS_REGION=\"us-east-1\"\nDEFANG_PROVIDER=\"aws\"\nDEFANG_RECIPE=\"affordable\"",
		},
		{
			name: "Unspecified mode",
			params: Parameters{
				Name:     "nomodestack",
				Provider: client.ProviderAWS,
				Region:   "us-west-1",
				Recipe:   modes.RecipeUnspecified,
			},
			expectedContent: "AWS_REGION=\"us-west-1\"\nDEFANG_PROVIDER=\"aws\"",
		},
		{
			name: "Empty region",
			params: Parameters{
				Name:     "noregionstack",
				Provider: client.ProviderGCP,
				Region:   "",
				Recipe:   modes.RecipeAffordable,
			},
			expectedContent: "DEFANG_PROVIDER=\"gcp\"\nDEFANG_RECIPE=\"affordable\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := Marshal(&tt.params)
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
		expectedParams Parameters
	}{
		{
			name: "GCP provider",
			content: `DEFANG_PROVIDER=gcp
GOOGLE_REGION=us-central1
DEFANG_MODE=BALANCED
`,
			expectedParams: Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Recipe:   modes.RecipeBalanced,
			},
		},
		{
			name: "AWS provider",
			content: `DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
DEFANG_MODE=AFFORDABLE
`,
			expectedParams: Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-east-1",
				Recipe:   modes.RecipeAffordable,
			},
		},
		{
			name: "Azure provider",
			content: `DEFANG_PROVIDER=azure
AZURE_LOCATION=eastus
DEFANG_MODE=STAGING
`,
			expectedParams: Parameters{
				Provider: client.ProviderAzure,
				Region:   "eastus",
				Recipe:   modes.RecipeBalanced,
			},
		},
		{
			name: "With recipe",
			content: `DEFANG_PROVIDER=aws
AWS_REGION=us-west-2
DEFANG_RECIPE=affordable
`,
			expectedParams: Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Recipe:   modes.RecipeAffordable,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := NewParametersFromContent(tt.name, []byte(tt.content))
			if err != nil {
				t.Errorf("Parse() error = %v", err)
				return
			}
			assert.Equal(t, tt.expectedParams.Provider, params.Provider)
			assert.Equal(t, tt.expectedParams.Region, params.Region)
			assert.Equal(t, tt.expectedParams.Recipe, params.Recipe)
		})
	}
}

func TestReadInDirectory(t *testing.T) {
	t.Run("read existing stack", func(t *testing.T) {
		t.Chdir(t.TempDir())
		// Create dummy stack file
		stackName := "stacktoread"
		expectedParams := Parameters{
			Name:     stackName,
			Provider: client.ProviderAWS,
			Region:   "us-west-2",
			Recipe:   modes.RecipeAffordable,
		}
		_, err := CreateInDirectory(".", expectedParams)
		if err != nil {
			t.Errorf("Setup CreateInDirectory() error = %v", err)
		}

		params, err := ReadInDirectory(".", stackName)
		if err != nil {
			t.Errorf("Read() error = %v", err)
		}
		if params.Provider != expectedParams.Provider ||
			params.Region != expectedParams.Region ||
			params.Recipe != expectedParams.Recipe {
			t.Errorf("Read() = %v, want %v", params, expectedParams)
		}
	})
}

func TestParamsToMap(t *testing.T) {
	tests := []struct {
		name        string
		params      Parameters
		expectedMap map[string]string
	}{
		{
			name: "AWS params",
			params: Parameters{
				Name:     "teststack",
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
				Recipe: modes.RecipeAffordable,
			},
			expectedMap: map[string]string{
				"DEFANG_PROVIDER": "aws",
				"AWS_REGION":      "us-west-2",
				"AWS_PROFILE":     "default",
				"DEFANG_RECIPE":   "affordable",
			},
		},
		{
			name: "GCP params",
			params: Parameters{
				Name:     "gcpstack",
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Variables: map[string]string{
					"GCP_PROJECT_ID": "gcp-project-123",
				},
				Recipe: modes.RecipeBalanced,
			},
			expectedMap: map[string]string{
				"DEFANG_PROVIDER": "gcp",
				"GOOGLE_REGION":   "us-central1",
				"GCP_PROJECT_ID":  "gcp-project-123",
				"DEFANG_RECIPE":   "balanced",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultMap := tt.params.ToMap()
			if len(resultMap) != len(tt.expectedMap) {
				t.Errorf("Params.ToMap() = %v, want %v", resultMap, tt.expectedMap)
			}
			for key, expectedValue := range tt.expectedMap {
				if resultMap[key] != expectedValue {
					t.Errorf("Params.ToMap()[%q] = %q, want %q", key, resultMap[key], expectedValue)
				}
			}
		})
	}
}

func TestParamsFromMap(t *testing.T) {
	tests := []struct {
		name           string
		inputMap       map[string]string
		expectedParams Parameters
	}{
		{
			name: "GCP params",
			inputMap: map[string]string{
				"DEFANG_PROVIDER": "gcp",
				"GOOGLE_REGION":   "us-central1",
				"DEFANG_RECIPE":   "balanced",
			},
			expectedParams: Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Recipe:   modes.RecipeBalanced,
			},
		},
		{
			name: "AWS params",
			inputMap: map[string]string{
				"DEFANG_PROVIDER": "aws",
				"AWS_REGION":      "us-west-2",
				"AWS_PROFILE":     "default",
				"DEFANG_RECIPE":   "affordable",
			},
			expectedParams: Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
				Recipe: modes.RecipeAffordable,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultParams, err := paramsFromMap(tt.inputMap)
			if err != nil {
				t.Errorf("ParamsFromMap() error = %v", err)
			}

			if resultParams.Provider != tt.expectedParams.Provider ||
				resultParams.Region != tt.expectedParams.Region ||
				resultParams.Recipe != tt.expectedParams.Recipe ||
				resultParams.Variables["AWS_PROFILE"] != tt.expectedParams.Variables["AWS_PROFILE"] {
				t.Errorf("ParamsFromMap() = %+v, want %+v", resultParams, tt.expectedParams)
			}
		})
	}
}
