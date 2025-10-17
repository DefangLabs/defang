package track

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkMessages(t *testing.T) {
	maxChunkSize := 5
	tests := []struct {
		name                  string
		prefix                string
		messages              []string
		expectedChunkContents []string
	}{
		{
			name:                  "empty messages",
			prefix:                "logs",
			messages:              []string{},
			expectedChunkContents: []string{},
		},
		{
			name:                  "single message",
			prefix:                "logs",
			messages:              []string{"msg"},
			expectedChunkContents: []string{"msg"},
		},
		{
			name:                  "three messages - one chunk",
			prefix:                "logs",
			messages:              []string{"1", "2", "3"},
			expectedChunkContents: []string{"1\n2\n3"},
		},
		{
			name:                  "four messages - 4 chunks",
			prefix:                "logs",
			messages:              []string{"123", "456", "789", "abc"},
			expectedChunkContents: []string{"123\n4", "56\n78", "9\nabc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChunkMessagesWithSize(tt.prefix, tt.messages, maxChunkSize)

			// Check number of chunks
			assert.Equal(t, len(tt.expectedChunkContents), len(result), "incorrect number of chunks")

			if len(tt.messages) == 0 {
				return // No more checks needed for empty input
			}

			// Verify chunk sizes and property names
			for i, prop := range result {
				// Check property name format
				expectedName := fmt.Sprintf("%s-%d", tt.prefix, i+1)
				assert.Equal(t, expectedName, prop.Name, "incorrect property name")

				// Check that value is string
				msgs, ok := prop.Value.(string)
				assert.True(t, ok, "property value should be string")

				// Check chunk size
				if i < len(result)-1 {
					// All chunks except the last should have maxMessagePerProperty messages
					assert.Equal(t, maxChunkSize, len(msgs), "non-final chunk has incorrect size")
				} else {
					// final chunk
					assert.LessOrEqual(t, len(msgs), maxChunkSize, "final chunk exceeds max size")
					assert.Greater(t, len(msgs), 0, "final chunk should not be empty")

					lastValue, ok := result[len(result)-1].Value.(string)
					assert.True(t, ok, "last chunk value should be string")
					assert.True(t, strings.HasSuffix(msgs, lastValue), "final chunk content mismatch")
				}
			}
		})
	}
}
