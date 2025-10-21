package evaluation

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
)

// Command represents command-line options for evaluation
type Command struct {
	Inputs     []string
	OutputPath string
	UseGenkit  bool
	Cluster    string
	AuthPort   int
	Provider   *client.ProviderID
	GenKit     *genkit.Genkit
}

// Execute runs the evaluation command
func (cmd *Command) Execute(ctx context.Context) error {
	config := &EvaluationConfig{
		Cluster:    cmd.Cluster,
		AuthPort:   cmd.AuthPort,
		ProviderId: cmd.Provider,
	}

	// Create evaluation runner
	runner, err := NewEvaluationRunner(config, cmd.GenKit)
	if err != nil {
		return fmt.Errorf("failed to create evaluation runner: %w", err)
	}

	// Validate required parameters
	if len(cmd.Inputs) == 0 {
		return errors.New("No input data, inputs are required")
	}

	flow := cmd.registerAgentFlow()

	// Run evaluation
	if cmd.UseGenkit {
		fmt.Println("Running evaluation using Genkit built-in functions...")
		return runner.RunEvaluationWithGenkitDataset(ctx, cmd.Inputs)
	} else {
		fmt.Println("Running custom evaluation...")
		resultGroup, err := runner.RunEvaluation(ctx, flow, cmd.Inputs, cmd.OutputPath)
		if err != nil {
			return err
		}

		// Print summary
		fmt.Printf("\n=== Evaluation Summary ===\n")
		for _, result := range resultGroup {
			fmt.Printf("Dataset: %s\n", result.DatasetID)
			fmt.Printf("Total Tests: %d\n", result.TotalTests)
			fmt.Printf("Passed: %d\n", result.PassedTests)
			fmt.Printf("Failed: %d\n", result.FailedTests)

			if result.Metrics != nil {
				fmt.Printf("Tool Selection Accuracy: %.2f%%\n", result.Metrics.ToolSelectionAccuracy*100)
				fmt.Printf("Response Quality: %.2f%%\n", result.Metrics.ResponseQuality*100)
				fmt.Printf("Error Rate: %.2f%%\n", result.Metrics.ErrorRate*100)
				fmt.Printf("Average Response Time: %.2f ms\n", result.Metrics.AverageResponseTime)
			}
		}

		return nil
	}
}

// registerAgentFlow registers the Defang agent as a flow for evaluation
func (cmd *Command) registerAgentFlow() *core.Flow[string, string, struct{}] {
	return genkit.DefineFlow(cmd.GenKit, "defangAgentFlow", func(ctx context.Context, input string) (string, error) {
		// Create a temporary agent instance for evaluation
		agentInstance := agent.New(ctx, cmd.Cluster, cmd.AuthPort, cmd.Provider)

		// Simulate handling the message like the interactive agent does
		return cmd.simulateAgentResponse(ctx, agentInstance, input)
	})
}

// simulateAgentResponse simulates the agent's message handling for evaluation
func (cmd *Command) simulateAgentResponse(ctx context.Context, agentInstance *agent.Agent, input string) (string, error) {
	// This is a simplified version of agent.handleMessage that returns the response as a string
	// instead of printing it to stdout

	// For now, return a placeholder - we'll implement this properly when we create the flow wrapper
	return "Agent response to: " + input, nil
}
