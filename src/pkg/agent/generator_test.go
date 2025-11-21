package agent

import (
	"context"
	"os"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
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
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	tests := []struct {
		name                     string
		maxTurns                 int
		generatorResponses       []*ai.ModelResponse
		expectedResponseMessages []*ai.Message
		expectedError            error
	}{
		{
			name:     "GenerateLoop no tool calls",
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
		{
			name:     "GenerateLoop with tool calls",
			maxTurns: 2,
			generatorResponses: []*ai.ModelResponse{
				{
					Message: ai.NewModelMessage(
						ai.NewToolRequestPart(&ai.ToolRequest{
							Name: "read_file",
							Input: map[string]any{
								"path": "value1",
							},
						}),
					),
				},
				{
					Message: ai.NewModelTextMessage("All done"),
				},
			},
			expectedResponseMessages: []*ai.Message{
				ai.NewUserTextMessage("User message"),
				ai.NewModelMessage(
					ai.NewToolRequestPart(&ai.ToolRequest{
						Name: "read_file",
						Input: map[string]any{
							"path": "value1",
						},
					}),
				),
				ai.NewMessage(ai.RoleTool, nil,
					ai.NewToolResponsePart(&ai.ToolResponse{
						Name:   "read_file",
						Ref:    "",
						Output: "error calling tool read_file: open " + cwd + "/value1: no such file or directory",
					}),
				),
				ai.NewModelMessage(
					ai.NewTextPart("All done"),
				),
			},
		},
		{
			name:     "GenerateLoop with tool calls in both responses",
			maxTurns: 2,
			generatorResponses: []*ai.ModelResponse{
				{
					Message: ai.NewModelMessage(
						ai.NewToolRequestPart(&ai.ToolRequest{
							Name: "read_file",
							Input: map[string]any{
								"path": "value1",
							},
						}),
					),
				},
				{
					Message: ai.NewModelMessage(
						ai.NewToolRequestPart(&ai.ToolRequest{
							Name: "read_file",
							Input: map[string]any{
								"path": "value2",
							},
						}),
					),
				},
			},
			expectedResponseMessages: []*ai.Message{
				ai.NewUserTextMessage("User message"),
				ai.NewModelMessage(
					ai.NewToolRequestPart(&ai.ToolRequest{
						Name: "read_file",
						Input: map[string]any{
							"path": "value1",
						},
					}),
				),
				ai.NewMessage(ai.RoleTool, nil,
					ai.NewToolResponsePart(&ai.ToolResponse{
						Name:   "read_file",
						Ref:    "",
						Output: "error calling tool read_file: open " + cwd + "/value1: no such file or directory",
					}),
				),
				ai.NewModelMessage(
					ai.NewToolRequestPart(&ai.ToolRequest{
						Name: "read_file",
						Input: map[string]any{
							"path": "value2",
						},
					}),
				),
				ai.NewMessage(ai.RoleTool, nil,
					ai.NewToolResponsePart(&ai.ToolResponse{
						Name:   "read_file",
						Ref:    "",
						Output: "error calling tool read_file: open " + cwd + "/value2: no such file or directory",
					}),
				),
			},
			expectedError: &maxTurnsReachedError{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Here you would set up the necessary context, prompt, and messages
			// For demonstration purposes, we'll use placeholders
			ctx := t.Context()
			printer := &mockPrinter{}

			gk := genkit.Init(ctx)
			toolManager := NewToolManager(gk, printer)
			fsTools := CollectFsTools()
			toolManager.RegisterTools(fsTools...)
			generator := &Generator{
				genkitGenerator: &mockGenkitGenerator{
					responses: tt.generatorResponses,
				},
				printer:     printer,
				toolManager: toolManager,
			}

			message := ai.NewUserTextMessage("User message")
			err := generator.HandleMessage(ctx, prompt, tt.maxTurns, message)
			if tt.expectedError != nil {
				assert.ErrorIs(t, err, tt.expectedError, "GenerateLoop should return the expected error")
			} else {
				assert.NoError(t, err, "GenerateLoop should not return an error")
			}
			for i, resp := range generator.messages {
				expectedContent := tt.expectedResponseMessages[i].Content[0]
				actualContent := resp.Content[0]
				assert.Equal(t, expectedContent.Kind, actualContent.Kind, "Response message part kind should match")
				assert.Equal(t, expectedContent.Text, actualContent.Text, "Response message should match expected")
				if expectedContent.ToolRequest != nil {
					assert.Equal(t, expectedContent.ToolRequest.Name, actualContent.ToolRequest.Name, "Tool request name should match")
				}
				if expectedContent.ToolResponse != nil {
					assert.Equal(t, expectedContent.ToolResponse.Name, actualContent.ToolResponse.Name, "Tool response name should match")
					assert.Equal(t, expectedContent.ToolResponse.Output, actualContent.ToolResponse.Output, "Tool response output should match")
				}
			}
		})
	}
}
