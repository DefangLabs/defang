package evaluation

// import (
// 	"context"
// 	"fmt"
// 	"os"
// 	"path/filepath"
// 	"testing"
// )

// // ExampleUsage demonstrates how to use the evaluation framework
// func ExampleUsage() {
// 	ctx := context.Background()

// 	// Create evaluation configuration
// 	config := &EvaluationConfig{
// 		Cluster:    "playground",
// 		AuthPort:   0,
// 		ProviderId: nil, // Use playground provider
// 	}

// 	// Create evaluation runner
// 	runner, err := NewEvaluationRunner(config)
// 	if err != nil {
// 		fmt.Printf("Failed to create evaluation runner: %v\n", err)
// 		return
// 	}

// 	// Create default dataset
// 	datasetPath := "defang_scenarios.json"
// 	if err := runner.CreateDefaultDataset(datasetPath); err != nil {
// 		fmt.Printf("Failed to create dataset: %v\n", err)
// 		return
// 	}

// 	// Run evaluation
// 	result, err := runner.RunEvaluation(ctx, datasetPath, "results.json")
// 	if err != nil {
// 		fmt.Printf("Evaluation failed: %v\n", err)
// 		return
// 	}

// 	fmt.Printf("Evaluation completed: %d/%d tests passed\n", result.PassedTests, result.TotalTests)
// }

// // CreateSampleDatasets creates sample datasets for testing in the testdata directory
// func CreateSampleDatasets(outputDir string) error {
// 	if err := os.MkdirAll(outputDir, 0755); err != nil {
// 		return fmt.Errorf("failed to create output directory: %w", err)
// 	}

// 	// Create dataset manager
// 	dm := NewDatasetManager()

// 	// Create basic commands dataset
// 	basicDataset := dm.CreateDataset("basic_commands", "Basic Commands", "Test basic Defang commands")
// 	basicExamples := []*DatasetExample{
// 		{
// 			Input:     "Deploy my application",
// 			Reference: "(?i)(deploy|deployment)",
// 			Context:   map[string]interface{}{"scenario": "deploy"},
// 		},
// 		{
// 			Input:     "Show my services",
// 			Reference: "(?i)(service|list)",
// 			Context:   map[string]interface{}{"scenario": "services"},
// 		},
// 		{
// 			Input:     "View logs",
// 			Reference: "(?i)(log|logs)",
// 			Context:   map[string]interface{}{"scenario": "logs"},
// 		},
// 	}
// 	for _, example := range basicExamples {
// 		dm.AddExample(basicDataset.ID, example)
// 	}

// 	// Save dataset
// 	basicPath := filepath.Join(outputDir, "basic_commands.json")
// 	if err := dm.SaveDatasetToFile(basicDataset.ID, basicPath); err != nil {
// 		return fmt.Errorf("failed to save basic commands dataset: %w", err)
// 	}

// 	fmt.Printf("Created dataset: %s\n", basicPath)
// 	return nil
// }

// // BenchmarkEvaluation benchmarks the evaluation framework performance
// func BenchmarkEvaluation(b *testing.B) {
// 	ctx := context.Background()

// 	config := &EvaluationConfig{
// 		Cluster:    "playground",
// 		AuthPort:   0,
// 		ProviderId: nil,
// 	}

// 	runner, err := NewEvaluationRunner(config)
// 	if err != nil {
// 		b.Fatalf("Failed to create runner: %v", err)
// 	}

// 	// Create a small test dataset
// 	dm := NewDatasetManager()
// 	dataset := dm.CreateDataset("benchmark", "Benchmark Dataset", "Performance testing")
// 	dm.AddExample(dataset.ID, &DatasetExample{
// 		Input:     "Deploy application",
// 		Reference: "(?i)deploy",
// 	})

// 	b.ResetTimer()

// 	for i := 0; i < b.N; i++ {
// 		_, err := runner.flowWrapper.EvaluateFlow(ctx, dataset)
// 		if err != nil {
// 			b.Fatalf("Evaluation failed: %v", err)
// 		}
// 	}
// }

// // TestEvaluationFramework provides a comprehensive test of the evaluation framework
// func TestEvaluationFramework(t *testing.T) {
// 	ctx := context.Background()

// 	// Test configuration
// 	config := &EvaluationConfig{
// 		Cluster:    "playground",
// 		AuthPort:   0,
// 		ProviderId: nil,
// 	}

// 	// Create framework
// 	framework, err := NewEvaluationFramework(ctx, config)
// 	if err != nil {
// 		t.Fatalf("Failed to create framework: %v", err)
// 	}

// 	// Test that framework is properly initialized
// 	if framework.GetGenkit() == nil {
// 		t.Error("Genkit instance is nil")
// 	}

// 	if framework.GetFlow() == nil {
// 		t.Error("Flow instance is nil")
// 	}

// 	// Test dataset creation
// 	dm := NewDatasetManager()
// 	dataset := dm.CreateDataset("test", "Test Dataset", "Testing")

// 	example := &DatasetExample{
// 		Input:     "Test input",
// 		Reference: "test",
// 		Context:   map[string]interface{}{"test": true},
// 	}

// 	if err := dm.AddExample(dataset.ID, example); err != nil {
// 		t.Errorf("Failed to add example: %v", err)
// 	}

// 	if len(dataset.Examples) != 1 {
// 		t.Errorf("Expected 1 example, got %d", len(dataset.Examples))
// 	}

// 	// Test evaluator creation
// 	evaluators := CreateDefangEvaluators()
// 	if len(evaluators) == 0 {
// 		t.Error("No evaluators created")
// 	}

// 	t.Logf("Successfully created %d evaluators", len(evaluators))
// }

// // QuickStart provides a quick start example for the evaluation framework
// func QuickStart() error {
// 	ctx := context.Background()

// 	fmt.Println("=== Defang Agent Evaluation Framework ===")
// 	fmt.Println("Quick Start Example")

// 	// Simple inputs to test
// 	inputs := []string{
// 		"Deploy my app",
// 		"Show services",
// 		"Get logs",
// 	}

// 	// Run quick evaluation
// 	if err := QuickEval(ctx, inputs, "playground", 0, nil); err != nil {
// 		return fmt.Errorf("quick evaluation failed: %w", err)
// 	}

// 	fmt.Println("Quick evaluation completed successfully!")
// 	return nil
// }

// // MainEvaluationCLI demonstrates the main CLI usage pattern
// func MainEvaluationCLI() error {
// 	ctx := context.Background()

// 	// Command-line style usage
// 	cmd := &EvaluationCommand{
// 		CreateDefault: true,
// 		OutputPath:    "my_dataset.json",
// 		Cluster:       "playground",
// 		AuthPort:      0,
// 	}

// 	// Create default dataset
// 	if err := cmd.Execute(ctx); err != nil {
// 		return fmt.Errorf("failed to create dataset: %w", err)
// 	}

// 	// Run evaluation
// 	cmd.CreateDefault = false
// 	cmd.DatasetPath = "my_dataset.json"
// 	cmd.OutputPath = "results.json"

// 	if err := cmd.Execute(ctx); err != nil {
// 		return fmt.Errorf("failed to run evaluation: %w", err)
// 	}

// 	fmt.Println("Evaluation completed! Check results.json for details.")
// 	return nil
// }
