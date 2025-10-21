package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/evaluation"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/evaluators"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

var (
	input    = flag.String("input", "", "Path to the input dataset")
	output   = flag.String("output", "evaluation_results.json", "Path to save evaluation results")
	provider = flag.String("provider", "defang", "Provider ID (aws, gcp, or defang)")
	help     = flag.Bool("help", false, "Show help message")
	dev      = flag.Bool("dev", false, "Enable development mode")
)

func main() {
	flag.Parse()

	// if help flag is set, show help
	if *help {
		showHelp()
		return
	}

	if *input == "" {
		fmt.Println("Error: input dataset path is required.")
		showHelp()
		return
	}

	inputFiles, err := getInputJSONFiles(*input)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if len(inputFiles) == 0 {
		fmt.Println("Error: no JSON files found at input path.")
		os.Exit(1)
	}

	log.Printf("input data: %v", inputFiles)

	if *provider == "" || !slices.Contains([]string{"aws", "gcp", "defang"}, strings.ToLower(*provider)) {
		fmt.Println("Error: provider is required and must be one of: aws, gcp, defang.")
		showHelp()
		return
	}

	ctx := context.Background()
	genkit, err := initGenKit(ctx, nil)
	if err != nil {
		log.Fatalf("Failed to initialize GenKit: %v", err)
	}

	// configure and execute evaluation command
	cmd := &evaluation.Command{
		Inputs:     inputFiles,
		OutputPath: *output,
		Provider:   toProviderId(strings.ToLower(*provider)),
		Cluster:    "beta",
		AuthPort:   0,
		UseGenkit:  false,
		GenKit:     genkit,
	}

	if err := cmd.Execute(ctx); err != nil {
		log.Fatalf("Evaluation failed: %v", err)
	}

	if *dev {
		// Keep the program running for inspection in dev mode
		select {}
	}
}

func toProviderId(provider string) *client.ProviderID {
	p := client.ProviderDefang
	switch strings.ToLower(provider) {
	case "aws":
		p = client.ProviderAWS
	case "gcp":
		p = client.ProviderGCP
	}

	return &p
}

func showHelp() {
	fmt.Println("Defang Agent Evaluation Framework")
	fmt.Println("=================================")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  go run main.go [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -input string")
	fmt.Println("        Path to dataset JSON file(s)")
	fmt.Println("  -output string")
	fmt.Println("        Path to save evaluation results (default \"evaluation_results.json\")")
	fmt.Println("  -provider string")
	fmt.Println("        Provider ID (aws, gcp, or empty for playground)")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Environment Variables:")
	fmt.Println("  GOOGLE_API_KEY or GEMINI_API_KEY - Required for real evaluation")
	fmt.Println("  Get your API key at: https://ai.google.dev")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Run evaluation (requires API key)")
	fmt.Println("  export GOOGLE_API_KEY=your_key_here")
	fmt.Println("  go run main.go -dataset defang_scenarios.json")
	fmt.Println()
	fmt.Println("  # Run simulation mode (no API key required)")
	fmt.Println("  go run main.go -input defang_scenarios.json")
}

func getInputJSONFiles(path string) ([]string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access input path: %w", err)
	}

	var jsonFiles []string
	if fileInfo.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read input directory: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && hasJSONExtension(entry.Name()) {
				jsonFiles = append(jsonFiles, fmt.Sprintf("%s/%s", path, entry.Name()))
			}
		}
	} else {
		if hasJSONExtension(fileInfo.Name()) {
			jsonFiles = append(jsonFiles, path)
		} else {
			return nil, fmt.Errorf("input file is not a JSON file")
		}
	}

	return jsonFiles, nil
}

func hasJSONExtension(filename string) bool {
	return len(filename) > 5 && filename[len(filename)-5:] == ".json"
}

// NewEvaluationFramework creates a new evaluation framework instance
func initGenKit(ctx context.Context, metricConfig []evaluators.MetricConfig) (*genkit.Genkit, error) {
	// Set default metrics if none provided
	if len(metricConfig) == 0 {
		metricConfig = []evaluators.MetricConfig{
			{MetricType: evaluators.EvaluatorRegex},
			{MetricType: evaluators.EvaluatorDeepEqual},
			{MetricType: evaluators.EvaluatorJsonata},
		}
	}

	// Initialize Genkit with plugins
	g := genkit.Init(ctx,
		genkit.WithPlugins(
			&googlegenai.GoogleAI{},
			&evaluators.GenkitEval{Metrics: metricConfig},
		),
		genkit.WithDefaultModel("googleai/gemini-2.5-flash"),
	)

	return g, nil
}
