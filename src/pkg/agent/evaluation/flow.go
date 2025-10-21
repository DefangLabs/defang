package evaluation

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

// AgentFlowWrapper wraps the Defang agent for evaluation purposes
type AgentFlowWrapper struct {
	cluster    string
	authPort   int
	providerId *client.ProviderID
	genkit     *genkit.Genkit
	tools      []ai.ToolRef
}

// NewAgentFlowWrapper creates a new agent flow wrapper for evaluation
func NewAgentFlowWrapper(cluster string, authPort int, providerId *client.ProviderID, g *genkit.Genkit) *AgentFlowWrapper {
	// Collect tools like the main agent does
	tools := agent.CollectTools(cluster, authPort, providerId)
	toolRefs := make([]ai.ToolRef, len(tools))
	for i, t := range tools {
		toolRefs[i] = ai.ToolRef(t)
	}

	return &AgentFlowWrapper{
		cluster:    cluster,
		authPort:   authPort,
		providerId: providerId,
		genkit:     g,
		tools:      toolRefs,
	}
}

// CreateEvaluationFlow creates a Genkit flow that wraps the agent's message handling
func (afw *AgentFlowWrapper) CreateEvaluationFlow() *core.Flow[string, string, struct{}] {
	return genkit.DefineFlow(afw.genkit, "defang-cli-agent", func(ctx context.Context, input string) (string, error) {
		return afw.HandleMessage(ctx, input)
	})
}

// HandleMessage processes a message through the agent and returns the response
func (afw *AgentFlowWrapper) HandleMessage(ctx context.Context, msg string) (string, error) {
	// Generate response using the same logic as the main agent
	resp, err := genkit.Generate(ctx, afw.genkit,
		ai.WithPrompt(msg),
		ai.WithTools(afw.tools...),
	)
	if err != nil {
		return "", fmt.Errorf("generation error: %w", err)
	}

	// Handle tool requests
	parts := []*ai.Part{}
	for _, req := range resp.ToolRequests() {
		tool := genkit.LookupTool(afw.genkit, req.Name)
		if tool == nil {
			log.Printf("tool %q not found", req.Name)
			continue
		}

		output, err := tool.RunRaw(ctx, req.Input)
		if err != nil {
			log.Printf("tool %q execution failed: %v", tool.Name(), err)
			// Continue with error response rather than failing completely
			output = fmt.Sprintf("Error executing tool %s: %v", req.Name, err)
		}

		parts = append(parts,
			ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: output,
			}))
	}

	// Generate final response if tools were used
	if len(parts) > 0 {
		resp, err = genkit.Generate(ctx, afw.genkit,
			ai.WithMessages(append(resp.History(), ai.NewMessage(ai.RoleTool, nil, parts...))...),
		)
		if err != nil {
			return "", fmt.Errorf("generation error: %w", err)
		}
	}

	// Extract text response from messages
	var responseText strings.Builder
	for _, msg := range resp.History() {
		if msg.Role == ai.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if part.Kind == ai.PartText {
				responseText.WriteString(part.Text)
				responseText.WriteString("\n")
			}
		}
	}

	return strings.TrimSpace(responseText.String()), nil
}

// EvaluationMetrics defines metrics for evaluating agent performance
type EvaluationMetrics struct {
	ToolSelectionAccuracy float64 `json:"tool_selection_accuracy"`
	ResponseQuality       float64 `json:"response_quality"`
	ErrorRate             float64 `json:"error_rate"`
	AverageResponseTime   float64 `json:"average_response_time"`
}

// FlowEvaluationResult contains results from evaluating the agent flow
type FlowEvaluationResult struct {
	DatasetID   string             `json:"dataset_id"`
	TotalTests  int                `json:"total_tests"`
	PassedTests int                `json:"passed_tests"`
	FailedTests int                `json:"failed_tests"`
	Metrics     *EvaluationMetrics `json:"metrics"`
	TestResults []*TestResult      `json:"test_results"`
	Timestamp   time.Time          `json:"timestamp"`
}

// TestResult represents the result of a single test case
type TestResult struct {
	Input          string                 `json:"input"`
	ExpectedOutput string                 `json:"expected_output"`
	ActualOutput   string                 `json:"actual_output"`
	Passed         bool                   `json:"passed"`
	Error          string                 `json:"error,omitempty"`
	ExecutionTime  float64                `json:"execution_time_ms"`
	Context        map[string]interface{} `json:"context,omitempty"`
}

// EvaluateFlow runs evaluation on the agent flow using the provided dataset
func (afw *AgentFlowWrapper) EvaluateFlow(ctx context.Context, flow *core.Flow[string, string, struct{}], datasets []*EvaluationDatasetInput) (*FlowEvaluationResult, error) {
	result := &FlowEvaluationResult{
		TotalTests:  len(datasets),
		TestResults: make([]*TestResult, 0, len(datasets)),
		Metrics:     &EvaluationMetrics{},
	}

	var totalExecutionTime float64
	var correctToolSelections int
	var errors int

	for _, example := range datasets {
		testResult := &TestResult{
			Input:          example.Input,
			ExpectedOutput: example.Reference,
			Context:        example.Context,
		}

		// Measure execution time
		start := time.Now()

		output, err := flow.Run(ctx, example.Input)

		end := time.Now()
		testResult.ExecutionTime = float64(end.Sub(start).Nanoseconds()) / 1000000 // Convert to milliseconds
		totalExecutionTime += testResult.ExecutionTime

		if err != nil {
			errors++
			testResult.Error = err.Error()
			testResult.Passed = false
		} else {
			testResult.ActualOutput = output

			// Evaluate if the test passed based on reference pattern (regex)
			if example.Reference != "" {
				testResult.Passed = afw.evaluateResponse(output, example.Reference)
				if testResult.Passed {
					result.PassedTests++
					correctToolSelections++
				} else {
					result.FailedTests++
				}
			}
		}

		result.TestResults = append(result.TestResults, testResult)
	}

	// Calculate metrics
	if result.TotalTests > 0 {
		result.Metrics.ToolSelectionAccuracy = float64(correctToolSelections) / float64(result.TotalTests)
		result.Metrics.ResponseQuality = float64(result.PassedTests) / float64(result.TotalTests)
		result.Metrics.ErrorRate = float64(errors) / float64(result.TotalTests)
		result.Metrics.AverageResponseTime = totalExecutionTime / float64(result.TotalTests)
	}

	return result, nil
}

// evaluateResponse evaluates whether the actual response matches the expected pattern
func (afw *AgentFlowWrapper) evaluateResponse(actual, expected string) bool {
	// For now, use simple string matching - this can be enhanced with regex or other evaluators
	return strings.Contains(strings.ToLower(actual), strings.ToLower(expected))
}
