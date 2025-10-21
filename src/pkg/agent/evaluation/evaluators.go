package evaluation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/firebase/genkit/go/ai"
)

// DefangToolSelectionEvaluator evaluates whether the agent selected the correct tools
type DefangToolSelectionEvaluator struct {
	name string
}

// NewDefangToolSelectionEvaluator creates a new tool selection evaluator
func NewDefangToolSelectionEvaluator() *DefangToolSelectionEvaluator {
	return &DefangToolSelectionEvaluator{
		name: "defang_tool_selection",
	}
}

// Name returns the evaluator name
func (e *DefangToolSelectionEvaluator) Name() string {
	return e.name
}

// Evaluate checks if the correct tool was used based on the input scenario
func (e *DefangToolSelectionEvaluator) Evaluate(ctx context.Context, req *ai.EvaluatorCallbackRequest) (*ai.EvaluatorCallbackResponse, error) {
	input, ok := req.Input.Input.(string)
	if !ok {
		return nil, errors.New("input is not a string")
	}

	output, ok := req.Input.Output.(string)
	if !ok {
		return nil, errors.New("output is not a string")
	}

	// Determine expected tool based on input keywords
	expectedTool := e.determineExpectedTool(input)
	actualTool := e.extractToolFromOutput(output)

	score := 0.0
	if expectedTool == actualTool {
		score = 1.0
	}

	result := ai.EvaluationResult{
		TestCaseId: req.Input.TestCaseId,
		Evaluation: []ai.Score{
			{
				Id:     "tool_selection_accuracy",
				Score:  score,
				Status: "PASS",
				Details: map[string]any{
					"expected_tool": expectedTool,
					"actual_tool":   actualTool,
					"match":         expectedTool == actualTool,
				},
			},
		},
	}

	return &result, nil
}

// determineExpectedTool analyzes input to determine which tool should be used
func (e *DefangToolSelectionEvaluator) determineExpectedTool(input string) string {
	input = strings.ToLower(input)

	// Define keywords for each tool
	toolKeywords := map[string][]string{
		"login":                   {"login", "authenticate", "signin", "sign in"},
		"deploy":                  {"deploy", "deployment", "start", "launch", "run"},
		"services":                {"services", "list services", "show services", "status"},
		"destroy":                 {"destroy", "delete", "remove", "stop", "tear down"},
		"logs":                    {"logs", "log", "show logs", "view logs"},
		"estimate":                {"estimate", "cost", "pricing", "price"},
		"set_config":              {"set config", "config set", "configure", "set variable"},
		"remove_config":           {"remove config", "delete config", "unset", "remove variable"},
		"list_configs":            {"list config", "show config", "config list", "configuration"},
		"set_aws_provider":        {"aws", "set aws", "aws provider"},
		"set_gcp_provider":        {"gcp", "google cloud", "set gcp"},
		"set_playground_provider": {"playground", "test", "demo"},
	}

	for tool, keywords := range toolKeywords {
		for _, keyword := range keywords {
			if strings.Contains(input, keyword) {
				return tool
			}
		}
	}

	return "unknown"
}

// extractToolFromOutput attempts to identify which tool was used from the output
func (e *DefangToolSelectionEvaluator) extractToolFromOutput(output string) string {
	output = strings.ToLower(output)

	// Look for tool-specific indicators in the output
	if strings.Contains(output, "login") || strings.Contains(output, "authentication") {
		return "login"
	}
	if strings.Contains(output, "deploy") || strings.Contains(output, "deployment") {
		return "deploy"
	}
	if strings.Contains(output, "services") || strings.Contains(output, "service list") {
		return "services"
	}
	if strings.Contains(output, "destroy") || strings.Contains(output, "deleted") {
		return "destroy"
	}
	if strings.Contains(output, "logs") || strings.Contains(output, "log entries") {
		return "logs"
	}
	if strings.Contains(output, "estimate") || strings.Contains(output, "cost") {
		return "estimate"
	}
	if strings.Contains(output, "config set") || strings.Contains(output, "configuration set") {
		return "set_config"
	}
	if strings.Contains(output, "config removed") || strings.Contains(output, "configuration removed") {
		return "remove_config"
	}
	if strings.Contains(output, "config list") || strings.Contains(output, "configurations") {
		return "list_configs"
	}

	return "unknown"
}

// DefangResponseQualityEvaluator evaluates the quality of agent responses
type DefangResponseQualityEvaluator struct {
	name string
}

// NewDefangResponseQualityEvaluator creates a new response quality evaluator
func NewDefangResponseQualityEvaluator() *DefangResponseQualityEvaluator {
	return &DefangResponseQualityEvaluator{
		name: "defang_response_quality",
	}
}

// Name returns the evaluator name
func (e *DefangResponseQualityEvaluator) Name() string {
	return e.name
}

// Evaluate checks the quality of the response
func (e *DefangResponseQualityEvaluator) Evaluate(ctx context.Context, req *ai.EvaluatorCallbackRequest) (*ai.EvaluatorCallbackResponse, error) {
	output, ok := req.Input.Output.(string)
	if !ok {
		return nil, errors.New("output is not a string")
	}

	score := e.calculateQualityScore(output)

	result := ai.EvaluationResult{
		TestCaseId: req.Input.TestCaseId,
		Evaluation: []ai.Score{
			{
				Id:     "response_quality",
				Score:  score,
				Status: "PASS",
				Details: map[string]any{
					"length":          len(output),
					"has_error":       e.hasError(output),
					"is_informative":  e.isInformative(output),
					"is_professional": e.isProfessional(output),
				},
			},
		},
	}

	return &result, nil
}

// calculateQualityScore computes an overall quality score for the response
func (e *DefangResponseQualityEvaluator) calculateQualityScore(output string) float64 {
	score := 0.0

	// Check if response has appropriate length (not too short, not too long)
	length := len(output)
	if length >= 10 && length <= 1000 {
		score += 0.25
	}

	// Check if response doesn't contain error indicators
	if !e.hasError(output) {
		score += 0.25
	}

	// Check if response is informative
	if e.isInformative(output) {
		score += 0.25
	}

	// Check if response is professional
	if e.isProfessional(output) {
		score += 0.25
	}

	return score
}

// hasError checks if the output contains error indicators
func (e *DefangResponseQualityEvaluator) hasError(output string) bool {
	errorPatterns := []string{
		"error", "failed", "failure", "exception", "panic", "fatal",
		"cannot", "unable", "not found", "invalid", "forbidden",
	}

	output = strings.ToLower(output)
	for _, pattern := range errorPatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

// isInformative checks if the response provides useful information
func (e *DefangResponseQualityEvaluator) isInformative(output string) bool {
	// Check for presence of useful information patterns
	informativePatterns := []string{
		"deployment", "service", "configuration", "status", "result",
		"complete", "success", "created", "updated", "removed",
		"available", "running", "stopped",
	}

	output = strings.ToLower(output)
	for _, pattern := range informativePatterns {
		if strings.Contains(output, pattern) {
			return true
		}
	}
	return false
}

// isProfessional checks if the response maintains a professional tone
func (e *DefangResponseQualityEvaluator) isProfessional(output string) bool {
	// Check for unprofessional language
	unprofessionalPatterns := []string{
		"damn", "shit", "fuck", "crap", "stupid", "dumb", "idiotic",
	}

	output = strings.ToLower(output)
	for _, pattern := range unprofessionalPatterns {
		if strings.Contains(output, pattern) {
			return false
		}
	}
	return true
}

// DefangRegexEvaluator evaluates responses using regex patterns
type DefangRegexEvaluator struct {
	name string
}

// NewDefangRegexEvaluator creates a new regex evaluator
func NewDefangRegexEvaluator() *DefangRegexEvaluator {
	return &DefangRegexEvaluator{
		name: "defang_regex",
	}
}

// Name returns the evaluator name
func (e *DefangRegexEvaluator) Name() string {
	return e.name
}

// Evaluate checks if the output matches the expected regex pattern
func (e *DefangRegexEvaluator) Evaluate(ctx context.Context, req *ai.EvaluatorCallbackRequest) (*ai.EvaluatorCallbackResponse, error) {
	output, ok := req.Input.Output.(string)
	if !ok {
		return nil, errors.New("output is not a string")
	}

	reference, ok := req.Input.Reference.(string)
	if !ok || reference == "" {
		return nil, errors.New("reference regex pattern is required")
	}

	// Compile and match regex
	regex, err := regexp.Compile(reference)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	matches := regex.MatchString(output)
	score := 0.0
	if matches {
		score = 1.0
	}

	status := "FAIL"
	if matches {
		status = "PASS"
	}

	result := ai.EvaluationResult{
		TestCaseId: req.Input.TestCaseId,
		Evaluation: []ai.Score{
			{
				Id:     "regex_match",
				Score:  score,
				Status: status,
				Details: map[string]any{
					"pattern": reference,
					"matches": matches,
					"output":  output,
				},
			},
		},
	}

	return &result, nil
}

// CreateDefangEvaluators creates instances of all Defang-specific evaluators
func CreateDefangEvaluators() []ai.Evaluator {
	return []ai.Evaluator{
		ai.NewEvaluator("defang_tool_selection", &ai.EvaluatorOptions{},
			NewDefangToolSelectionEvaluator().Evaluate),
		ai.NewEvaluator("defang_response_quality", &ai.EvaluatorOptions{},
			NewDefangResponseQualityEvaluator().Evaluate),
		ai.NewEvaluator("defang_regex", &ai.EvaluatorOptions{},
			NewDefangRegexEvaluator().Evaluate),
	}
}
