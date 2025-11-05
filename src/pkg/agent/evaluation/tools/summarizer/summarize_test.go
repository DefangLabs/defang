package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadResultsSchema(t *testing.T) {
	evalResults, err := LoadEvaluationResult("../../testdata/evaluation_results.json")
	if err != nil {
		t.Fatalf("Failed to load evaluation results: %v", err)
	}

	// Add assertions to check the contents of evalResults
	assert.Equal(t, len(evalResults.Results), 5, "Expected 5 evaluation results")
	assert.Equal(t, len(evalResults.Results[0].Metrics), 1, "First test case should have 1 metrics")
}

func TestLoadResultsSchemaFail(t *testing.T) {
	_, err := LoadEvaluationResult("../../testdata/no_file.json")
	if err == nil {
		t.Fatalf("Expected error but got none")
	}
}

func TestResultsScore(t *testing.T) {
	var testEvaluationSummary TestEvaluationSummary = SummarizeEvaluationResults("../../testdata")

	// these inputs correspond to the testdata/evaluation_results*.json test cases inputs
	inputs := []string{
		"Deploy my application but I'm not logged in",
		"Show services for a non-existent project",
		"Set an invalid configuration value",
		"Remove a config that doesn't exist",
		"Deploy without a valid compose file",
	}

	assert.Equal(t, len(inputs), len(testEvaluationSummary), "Expected summary for all test cases")

	assert.Equal(t, 2, testEvaluationSummary[inputs[0]].TotalTests, "Unexpected total tests for input 0")
	assert.Equal(t, 1, testEvaluationSummary[inputs[0]].Passed, "Unexpected passed tests for input 0")

	assert.Equal(t, 2, testEvaluationSummary[inputs[1]].TotalTests, "Unexpected total tests for input 1")
	assert.Equal(t, 0, testEvaluationSummary[inputs[1]].Passed, "Unexpected passed tests for input 1")

	assert.Equal(t, 2, testEvaluationSummary[inputs[2]].TotalTests, "Unexpected total tests for input 2")
	assert.Equal(t, 1, testEvaluationSummary[inputs[2]].Passed, "Unexpected passed tests for input 2")
}
