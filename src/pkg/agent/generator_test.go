package agent

import (
	"context"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/stretchr/testify/assert"
)

// create a mock GenkitGenerator for testing
type mockGenkitGenerator struct {
	responses []*ai.ModelResponse
	err       error
	callCount int
}

func (m *mockGenkitGenerator) Generate(ctx context.Context, prompt string, tools []ai.ToolRef, messages []*ai.Message, streamingCallback func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error) {
	if m.callCount < len(m.responses) {
		resp := m.responses[m.callCount]
		m.callCount++
		return resp, m.err
	}
	return nil, m.err
}

func TestHandleMessage(t *testing.T) {
	prompt := "Test prompt"
	tests := []struct {
		name                     string
		maxTurns                 int
		generatorResponses       []*ai.ModelResponse
		expectedResponseMessages []*ai.Message
		expectedError            error
	}{
		{
			name:     "HandleMessage no tool calls",
			maxTurns: 2,
			generatorResponses: []*ai.ModelResponse{
				{
					Message: ai.NewModelTextMessage("Response 1"),
				},
			},
			expectedResponseMessages: []*ai.Message{
				ai.NewUserTextMessage("User message"),
				ai.NewModelTextMessage("Response 1"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			printer := &mockPrinter{}

			generator := &Generator{
				genkitGenerator: &mockGenkitGenerator{
					responses: tt.generatorResponses,
				},
				printer: printer,
			}

			message := ai.NewUserTextMessage("User message")
			err := generator.HandleMessage(ctx, prompt, tt.maxTurns, message)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError, "HandleMessage should return the expected error")
			} else {
				assert.NoError(t, err, "HandleMessage should not return an error")
			}
			for i, resp := range generator.messages {
				expectedContent := tt.expectedResponseMessages[i].Content[0]
				actualContent := resp.Content[0]
				assert.Equal(t, expectedContent.Kind, actualContent.Kind, "Response message part kind should match")
				assert.Equal(t, expectedContent.Text, actualContent.Text, "Response message should match expected")
			}
		})
	}
}
