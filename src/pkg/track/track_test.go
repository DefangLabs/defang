package track

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkMessages(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		messages []string
		want     int // number of expected chunks
	}{
		{
			name:     "empty messages",
			prefix:   "logs",
			messages: []string{},
			want:     0,
		},
		{
			name:     "single message",
			prefix:   "logs",
			messages: []string{"message1"},
			want:     1,
		},
		{
			name:     "three messages - one chunk",
			prefix:   "logs",
			messages: []string{"msg1", "msg2", "msg3"},
			want:     1,
		},
		{
			name:     "four messages - two chunks",
			prefix:   "logs",
			messages: []string{"msg1", "msg2", "msg3", "msg4"},
			want:     2,
		},
		{
			name:     "six messages - two full chunks",
			prefix:   "logs",
			messages: []string{"msg1", "msg2", "msg3", "msg4", "msg5", "msg6"},
			want:     2,
		},
		{
			name:     "seven messages - three chunks",
			prefix:   "debug",
			messages: []string{"msg1", "msg2", "msg3", "msg4", "msg5", "msg6", "msg7"},
			want:     3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ChunkMessages(tt.prefix, tt.messages)

			// Check number of chunks
			assert.Equal(t, tt.want, len(result), "incorrect number of chunks")

			if len(tt.messages) == 0 {
				return // No more checks needed for empty input
			}

			// Verify chunk sizes and property names
			totalMessages := 0
			for i, prop := range result {
				// Check property name format
				expectedName := fmt.Sprintf("%s-%d", tt.prefix, i+1)
				assert.Equal(t, expectedName, prop.Name, "incorrect property name")

				// Check that value is []string
				msgs, ok := prop.Value.([]string)
				assert.True(t, ok, "property value should be []string")

				// Check chunk size
				if i < len(result)-1 {
					// All chunks except the last should have maxMessagePerProperty messages
					assert.Equal(t, maxMessagePerProperty, len(msgs), "non-final chunk has incorrect size")
				} else {
					// Last chunk should have remaining messages (1-3 messages)
					expectedLastSize := len(tt.messages) - (i * maxMessagePerProperty)
					assert.Equal(t, expectedLastSize, len(msgs), "final chunk has incorrect size")
					assert.LessOrEqual(t, len(msgs), maxMessagePerProperty, "final chunk exceeds max size")
					assert.Greater(t, len(msgs), 0, "final chunk should not be empty")
				}

				totalMessages += len(msgs)
			}

			// Verify all messages are preserved
			assert.Equal(t, len(tt.messages), totalMessages, "total message count should be preserved")
		})
	}
}
