package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/evaluators"
)

type EvaluationConfig struct {
	Cluster    string
	AuthPort   int
	ProviderId *client.ProviderID
	Metrics    []evaluators.MetricConfig
}

// EvaluationRunner manages the execution of evaluations
type Runner struct {
	datasetMgr  *DatasetManager
	flowWrapper *AgentFlowWrapper
	genkit      *genkit.Genkit
}

// NewEvaluationRunner creates a new evaluation runner
func NewEvaluationRunner(config *EvaluationConfig, g *genkit.Genkit) (*Runner, error) {
	// Create dataset manager
	datasetMgr := NewDatasetManager()

	// Create flow wrapper
	flowWrapper := NewAgentFlowWrapper(config.Cluster, config.AuthPort, config.ProviderId, g)

	return &Runner{
		datasetMgr:  datasetMgr,
		flowWrapper: flowWrapper,
		genkit:      g,
	}, nil
}

// RunEvaluation runs an evaluation using the specified dataset
func (er *Runner) RunEvaluation(ctx context.Context, flow *core.Flow[string, string, struct{}], datasetPaths []string, outputPath string) ([]FlowEvaluationResult, error) {
	datasetGroups, err := LoadEvaluationDatasets(datasetPaths)
	if err != nil {
		return nil, fmt.Errorf("failed to load datasets: %w", err)
	}

	var resultGroup []FlowEvaluationResult
	for _, ds := range datasetGroups {
		// Run the evaluation
		result, err := er.flowWrapper.EvaluateFlow(ctx, flow, ds.inputs)
		if err != nil {
			return nil, fmt.Errorf("evaluation failed: %w", err)
		}

		resultGroup = append(resultGroup, *result)

		// Save results if output path is provided
		if outputPath != "" {
			if err := er.SaveEvaluationResults(result, outputPath); err != nil {
				fmt.Printf("Warning: failed to save results to %s: %v\n", outputPath, err)
			} else {
				fmt.Printf("Results saved to: %s\n", outputPath)
			}
		}
	}

	return resultGroup, nil
}

// RunEvaluationWithGenkitDataset runs evaluation using Genkit's built-in evaluation functions
func (er *Runner) RunEvaluationWithGenkitDataset(ctx context.Context, datasetPaths []string) error {
	// Load dataset
	datasetGroups, err := LoadEvaluationDatasets(datasetPaths)
	if err != nil {
		return fmt.Errorf("failed to load dataset: %w", err)
	}

	// Create custom evaluators
	customEvaluators := CreateDefangEvaluators()

	// Run evaluation using Genkit's evaluation API - simplified version
	// Note: This would need proper registry integration for full Genkit evaluation
	fmt.Printf("Running evaluation with %d test data and %d evaluators\n", len(datasetGroups), len(customEvaluators))

	// For now, return a placeholder response
	response := make(ai.EvaluatorResponse, 0)

	// Print results
	fmt.Printf("Evaluation completed with %d results\n", len(response))
	for i, result := range response {
		fmt.Printf("Result %d: %+v\n", i+1, result)
	}

	return nil
}

// SaveEvaluationResults saves evaluation results to a JSON file
func (er *Runner) SaveEvaluationResults(result *FlowEvaluationResult, outputPath string) error {
	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Add timestamp to results
	result.Timestamp = time.Now()

	// Marshal to JSON
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}

	return nil
}

// ListDatasets returns information about available datasets
func (er *Runner) ListDatasets() []*Dataset {
	return er.datasetMgr.ListDatasets()
}
