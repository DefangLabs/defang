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
	return &Runner{
		ctx:   ctx,
		g:     g,
		tools: toolRefs,
	}
}

// CreateEvaluationFlow creates a Genkit flow for evaluation purposes
func (r *Runner) CreateEvaluationFlow() *core.Flow[FlowInput, string, struct{}] {
	return genkit.DefineFlow(r.g, "defang-cli", func(ctx context.Context, input FlowInput) (string, error) {
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
		return r.HandleMessageForEvaluation(messageStr)
	})
}

// HandleMessageForEvaluation processes a message and returns the response for evaluation purposes
func (r *Runner) HandleMessageForEvaluation(msg string) (string, error) {
	log.Println("Function invoked: genkit.Generate")
	resp, err := genkit.Generate(r.ctx, r.g,
		ai.WithPrompt(msg),
		ai.WithTools(r.tools...),
		ai.WithReturnToolRequests(true),
	)
	if err != nil {
		return "", fmt.Errorf("Generate error: %w", err)
	}

	toolsCalled := []string{}
	log.Printf("Tools called: %d", len(resp.ToolRequests()))
	for _, req := range resp.ToolRequests() {
		toolsCalled = append(toolsCalled, strings.ToLower(req.Name))
	}

	return fmt.Sprintf("Tools[%s]", strings.Join(toolsCalled, ", ")), nil
}
