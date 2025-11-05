package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/evaluators"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type FlowConfig struct {
	Cluster        string
	AuthPort       int
	EvaluationMode bool
	AIModel        string
	ProviderId     *client.ProviderID
	EvalMetrics    []evaluators.MetricConfig
}

type FlowSetup struct {
	WorkingDirectory *string `json:"working_directory,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Region           *string `json:"region,omitempty"`
}

type FlowInput struct {
	Message string    `json:"message"`
	Setup   FlowSetup `json:"setup,omitempty"`
}

type Runner struct {
	ctx   context.Context
	g     *genkit.Genkit
	tools []ai.ToolRef
}

func NewFlowRunner(ctx context.Context, config FlowConfig, tools []ai.Tool) *Runner {
	// Initialize Genkit with the Google AI plugin
	g := genkit.Init(ctx,
		genkit.WithPlugins(
			&googlegenai.GoogleAI{},
			&evaluators.GenkitEval{Metrics: config.EvalMetrics},
		),
		genkit.WithDefaultModel(config.AIModel),
	)

	toolRefs := make([]ai.ToolRef, len(tools))
	for i, t := range tools {
		toolRefs[i] = ai.ToolRef(t)
	}

	if g == nil {
		log.Fatalf("Genkit initialization failed: returned nil. Check AIModel and plugin configuration.")
	}

	return &Runner{
		ctx:   ctx,
		g:     g,
		tools: toolRefs,
	}
}

// CreateEvaluationFlow creates a Genkit flow for evaluation purposes
func (r *Runner) CreateEvaluationFlow() *core.Flow[FlowInput, []string, struct{}] {
	return genkit.DefineFlow(r.g, "defang-cli", func(ctx context.Context, input FlowInput) ([]string, error) {
		message := []string{}
		if input.Setup.WorkingDirectory != nil && *input.Setup.WorkingDirectory != "" {
			message = append(message, fmt.Sprintf("Make the working directory \"%s\"", *input.Setup.WorkingDirectory))
		}
		if input.Setup.Provider != nil && *input.Setup.Provider != "" {
			message = append(message, fmt.Sprintf("Set the provider to %s", *input.Setup.Provider))
		}
		if input.Setup.Region != nil && *input.Setup.Region != "" {
			message = append(message, fmt.Sprintf("Set the region to %s", *input.Setup.Region))
		}

		message = append(message, input.Message)
		messageStr := strings.Join(message, ". ")
		log.Printf("Flow input: %s", messageStr)

		result, err := r.HandleMessageForEvaluation(messageStr)
		if err != nil {
			return []string{}, err
		}

		// Return as array with single element to match expected schema
		return []string{result}, nil
	})
}

// HandleMessageForEvaluation processes a message and returns the response for evaluation purposes
func (r *Runner) HandleMessageForEvaluation(msg string) (string, error) {
	log.Printf("HandleMessageForEvaluation called with: %s", msg)

	allToolsCalled := []string{}
	maxRounds := 5

	// First generation call
	resp, err := genkit.Generate(r.ctx, r.g,
		ai.WithPrompt(msg),
		ai.WithTools(r.tools...),
		ai.WithReturnToolRequests(true),
	)
	if err != nil {
		log.Printf("Generate error: %v", err)
		return "Tools[]", fmt.Errorf("Generate error: %w", err)
	}

	if resp == nil {
		log.Printf("Warning: received nil response from Generate")
		return "Tools[]", nil
	}

	toolRequests := resp.ToolRequests()
	if toolRequests == nil {
		toolRequests = []*ai.ToolRequest{}
	}
	log.Printf("Initial response: %d tool requests, text: %q", len(toolRequests), resp.Text())

	// Multi-round conversation
	for round := 2; round <= maxRounds && len(toolRequests) > 0; round++ {
		// Process tool requests and build responses
		parts := []*ai.Part{}
		roundTools := []string{}

		for _, req := range toolRequests {
			toolName := strings.ToLower(req.Name)
			roundTools = append(roundTools, toolName)
			allToolsCalled = append(allToolsCalled, toolName)

			log.Printf("Tool request: %s", req.Name)

			// Simulate tool response for evaluation
			toolResp := &ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: fmt.Sprintf("Tool %s executed successfully.", req.Name),
			}
			parts = append(parts, ai.NewToolResponsePart(toolResp))
		}

		log.Printf("Round %d: called tools %v", round-1, roundTools)

		// Continue conversation with tool responses
		history := resp.History()
		if history == nil {
			history = []*ai.Message{}
		}

		if r.g == nil {
			log.Printf("Genkit instance is nil in round %d", round)
			break
		}

		metadata := map[string]any{}
		resp, err = genkit.Generate(r.ctx, r.g,
			ai.WithMessages(append(history, ai.NewMessage(ai.RoleTool, metadata, parts...))...),
		)
		if err != nil {
			log.Printf("Generate error in round %d: %v", round, err)
			break
		}

		if resp == nil {
			log.Printf("Warning: nil response in round %d", round)
			break
		}

		toolRequests = resp.ToolRequests()
		if toolRequests == nil {
			toolRequests = []*ai.ToolRequest{}
		}
	}

	if len(allToolsCalled) > 0 {
		result := fmt.Sprintf("Tools[%s]", strings.Join(allToolsCalled, ", "))
		log.Printf("Returning tools called: %s", result)
		return result, nil
	}

	// Always return a valid Tools[] format, never empty string
	log.Printf("No tools called, returning empty Tools[]")
	return "Tools[]", nil
}
