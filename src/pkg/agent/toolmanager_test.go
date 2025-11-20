package agent

import (
	"os"
	"testing"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/stretchr/testify/assert"
)

func TestHandleToolCalls(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	gk := genkit.Init(t.Context())
	printer := &mockPrinter{}
	toolManager := NewToolManager(gk, printer)
	fsTools := CollectFsTools()
	toolManager.RegisterTools(fsTools...)
	tests := []struct {
		name          string
		toolRequests  []*ai.ToolRequest
		expectedReply *ai.Message
	}{
		{
			name: "valid read_file tool call",
			toolRequests: []*ai.ToolRequest{
				{
					Name: "read_file",
					Input: map[string]any{
						"path": "somefile.txt",
					},
				},
			},
			expectedReply: ai.NewMessage(
				ai.RoleTool,
				nil,
				ai.NewToolResponsePart(&ai.ToolResponse{
					Name:   "read_file",
					Ref:    "",
					Output: "error calling tool read_file: open " + cwd + "/somefile.txt: no such file or directory", // Assuming the tool returns this content
				}),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := toolManager.HandleToolCalls(t.Context(), tt.toolRequests)
			assert.Equal(t, tt.expectedReply.Role, reply.Role, "Reply role should match")
			assert.Len(t, reply.Content, len(tt.expectedReply.Content), "Reply content length should match")
			for i, expectedContent := range tt.expectedReply.Content {
				actualContent := reply.Content[i]
				assert.Equal(t, expectedContent.Kind, actualContent.Kind, "Reply content part kind should match")
				assert.Equal(t, expectedContent.Text, actualContent.Text, "Reply content text should match")
				if expectedContent.ToolResponse != nil {
					assert.Equal(t, expectedContent.ToolResponse.Name, actualContent.ToolResponse.Name, "Tool response name should match")
					assert.Equal(t, expectedContent.ToolResponse.Output, actualContent.ToolResponse.Output, "Tool response output should match")
				}
			}
		})
	}
}

// Test ToolManager infinite loop detection
func TestToolManager_InfiniteLoopDetection_TableDriven(t *testing.T) {
	tests := []struct {
		name           string
		toolRequests   [][]*ai.ToolRequest
		expectedErrors []bool
	}{
		{
			name: "repeated request triggers error",
			toolRequests: [][]*ai.ToolRequest{
				{
					{
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value"},
					},
				},
				{
					{
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value"},
					},
				},
			},
			expectedErrors: []bool{false, true},
		},
		{
			name: "different inputs do not trigger error",
			toolRequests: [][]*ai.ToolRequest{
				{
					{
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value1"},
					},
				},
				{
					{
						Name:  "test_tool",
						Input: map[string]interface{}{"param": "value2"},
					},
				},
			},
			expectedErrors: []bool{false, false},
		},
		{
			name: "different tools do not trigger error",
			toolRequests: [][]*ai.ToolRequest{
				{
					{
						Name:  "test_tool_1",
						Input: map[string]interface{}{"param": "value"},
					},
				},
				{
					{
						Name:  "test_tool_2",
						Input: map[string]interface{}{"param": "value"},
					},
				},
			},
			expectedErrors: []bool{false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toolManager := &ToolManager{
				prevTurnToolRequestsJSON: make(map[string]bool),
			}
			for i, reqs := range tt.toolRequests {
				err := toolManager.EqualPrevious(reqs)
				if tt.expectedErrors[i] {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			}
		})
	}
}
