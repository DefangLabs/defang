package elicitations

// write tests for questionFromSchemaProp
import (
	"testing"

	"github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuestionFromSchemaProp(t *testing.T) {
	tests := []struct {
		name          string
		key           string
		propMap       map[string]any
		expectedQ     *survey.Question
		expectedError string
	}{
		{
			name: "string_type",
			key:  "username",
			propMap: map[string]any{
				"type":        "string",
				"description": "Enter your username",
			},
			expectedQ: &survey.Question{
				Name: "username",
				Prompt: &survey.Input{
					Message: "Enter your username",
				},
			},
		},
		{
			name: "string_type_with_default",
			key:  "username",
			propMap: map[string]any{
				"type":        "string",
				"description": "Enter your username",
				"default":     "admin",
			},
			expectedQ: &survey.Question{
				Name: "username",
				Prompt: &survey.Input{
					Message: "Enter your username",
					Default: "admin",
				},
			},
		},
		{
			name: "enum_type",
			key:  "color",
			propMap: map[string]any{
				"type":        "string",
				"description": "Choose a color",
				"enum":        []string{"red", "green", "blue"},
			},
			expectedQ: &survey.Question{
				Name: "color",
				Prompt: &survey.Select{
					Message: "Choose a color",
					Options: []string{"red", "green", "blue"},
				},
			},
		},
		{
			name: "invalid_enum_type",
			key:  "color",
			propMap: map[string]any{
				"type":        "string",
				"description": "Choose a color",
				"enum":        "not-an-array",
			},
			expectedError: "invalid enum values",
		},
		{
			name: "missing_description",
			key:  "email",
			propMap: map[string]any{
				"type": "string",
			},
			expectedQ: &survey.Question{
				Name: "email",
				Prompt: &survey.Input{
					Message: "email",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := questionFromSchemaProp(tt.key, tt.propMap)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedQ, q)
			}
		})
	}
}
