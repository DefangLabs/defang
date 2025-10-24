package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EvaluationResult represents the complete Genkit evaluation result structure
type EvaluationResult struct {
	Key             EvaluationKey             `json:"key"`
	Results         []TestCaseResult          `json:"results"`
	MetricsMetadata map[string]MetricMetadata `json:"metricsMetadata"`
}

// EvaluationKey contains metadata about the evaluation run
type EvaluationKey struct {
	EvalRunID       string          `json:"evalRunId"`
	CreatedAt       time.Time       `json:"createdAt"`
	MetricSummaries []MetricSummary `json:"metricSummaries"`
}

// MetricSummary provides aggregated results for each evaluator
type MetricSummary struct {
	Evaluator           string         `json:"evaluator"`
	TestCaseCount       int            `json:"testCaseCount"`
	ErrorCount          int            `json:"errorCount"`
	ScoreUndefinedCount int            `json:"scoreUndefinedCount"`
	StatusDistribution  map[string]int `json:"statusDistribution"`
}

// TestCaseResult represents the result of a single test case
type TestCaseResult struct {
	TestCaseID string    `json:"testCaseId"`
	Input      TestInput `json:"input"`
	Reference  string    `json:"reference"`
	TraceIDs   []string  `json:"traceIds"`
	Metrics    []Metric  `json:"metrics"`
}

// TestInput represents the input for a test case
type TestInput struct {
	Message string    `json:"message"`
	Setup   TestSetup `json:"setup,omitempty"`
}

// TestSetup contains configuration for the test environment
type TestSetup struct {
	WorkingDirectory *string `json:"working_directory,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Region           *string `json:"region,omitempty"`
}

// Metric represents an evaluation metric result
type Metric struct {
	Evaluator string   `json:"evaluator"`
	Status    string   `json:"status"` // "PASS" or "FAIL"
	Error     *string  `json:"error,omitempty"`
	TraceID   string   `json:"traceId"`
	SpanID    string   `json:"spanId"`
	Score     *float64 `json:"score,omitempty"`
}

// MetricMetadata provides information about an evaluator
type MetricMetadata struct {
	DisplayName string `json:"displayName"`
	Definition  string `json:"definition"`
}

type EvaluationSummary struct {
	TotalTests int
	Passed     int
}

type TestEvaluationSummary map[string]EvaluationSummary

// Common evaluator types
const (
	EvaluatorDeepEqual = "genkitEval/deep_equal"
	EvaluatorRegex     = "genkitEval/regex"
)

// Common status values
const (
	StatusPass = "PASS"
	StatusFail = "FAIL"
)

func SummarizeEvaluationResults(path string) TestEvaluationSummary {
	//get full path of all json files starting from path
	jsonFiles, err := findAllJSONFiles(path)
	if err != nil {
		return make(TestEvaluationSummary) // Return empty summary on error
	}

	summary := make(TestEvaluationSummary)

	for _, jsonFile := range jsonFiles {
		evalResult, err := LoadEvaluationResult(jsonFile)
		if err != nil {
			continue // Skip files that can't be loaded
		}

		// Process each test case in the evaluation result
		for _, testCase := range evalResult.Results {
			input := testCase.Input.Message

			if _, exists := summary[input]; !exists {
				summary[input] = EvaluationSummary{
					TotalTests: 0,
					Passed:     0,
				}
			}

			// all metrics must pass to be counted as successful
			allPassed := true
			for _, metric := range testCase.Metrics {
				allPassed = allPassed && (metric.Status == StatusPass)
				if !allPassed {
					break
				}
			}

			if allPassed {
				summary[input] = EvaluationSummary{
					TotalTests: summary[input].TotalTests + 1,
					Passed:     summary[input].Passed + 1,
				}
			} else {
				summary[input] = EvaluationSummary{
					TotalTests: summary[input].TotalTests + 1,
					Passed:     summary[input].Passed,
				}
			}
		}
	}

	return summary
}

// findAllJSONFiles recursively finds all JSON files starting from the given path
func findAllJSONFiles(rootPath string) ([]string, error) {
	var jsonFiles []string

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Check if it's a regular file and has .json extension
		if !d.IsDir() && strings.HasSuffix(strings.ToLower(path), ".json") {
			jsonFiles = append(jsonFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return jsonFiles, nil
}

func LoadEvaluationResult(path string) (*EvaluationResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var evalResults EvaluationResult
	err = json.Unmarshal(data, &evalResults)
	if err != nil {
		return nil, err
	}

	return &evalResults, nil
}
