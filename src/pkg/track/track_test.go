package track

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventMessages(t *testing.T) {
	tests := []struct {
		name                  string
		prefix                string
		messages              []string
		expectedEventContents []string
	}{
		{
			name:                  "empty messages",
			prefix:                "logs",
			messages:              []string{},
			expectedEventContents: []string{},
		},
		{
			name:                  "single message",
			prefix:                "logs",
			messages:              []string{"msg"},
			expectedEventContents: []string{"msg"},
		},
		{
			name:                  "three messages - three events",
			prefix:                "logs",
			messages:              []string{"1", "2", "3"},
			expectedEventContents: []string{"1", "2", "3"},
		},
		{
			name:                  "long message- truncated",
			prefix:                "logs",
			messages:              []string{"012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789"},
			expectedEventContents: []string{"012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MakeEventLogProperties(tt.prefix, tt.messages)

			// Check number of event properties created
			assert.Equal(t, len(tt.expectedEventContents), len(result), "incorrect number of event properties")

			if len(tt.messages) == 0 {
				return // No more checks needed for empty input
			}

			// Verify event properties
			for i, prop := range result {
				// Check property name format
				expectedName := fmt.Sprintf("%s-%d", tt.prefix, i+1)
				assert.Equal(t, expectedName, prop.Name, "incorrect property name")

				// Check that value is string
				propValue, ok := prop.Value.(string)
				assert.True(t, ok, "property value should be string")

				// Check size
				if len(tt.messages[i]) > maxPropertyCharacterLength && len(propValue) != maxPropertyCharacterLength {
					assert.Less(t, len(propValue), maxPropertyCharacterLength, "property value exceeds maxPropertyCharacterLength at %d", len(propValue))
				}
			}
		})
	}
}
