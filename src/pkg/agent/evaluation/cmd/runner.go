package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent"
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
func (r *Runner) CreateEvaluationFlow() *core.Flow[agent.DefangCLIInput, string, struct{}] {
	return genkit.DefineFlow(r.g, "defang-cli", func(ctx context.Context, input agent.DefangCLIInput) (string, error) {
		setupString := ""
		if input.Setup.WorkingDirectory != nil && *input.Setup.WorkingDirectory != "" {
			setupString += fmt.Sprintf("Make the current directory %s. ", *input.Setup.WorkingDirectory)
		}
		if input.Setup.Provider != nil && *input.Setup.Provider != "" {
			setupString += fmt.Sprintf("Use the %s provider. ", *input.Setup.Provider)
		}
		if input.Setup.Region != nil && *input.Setup.Region != "" {
			setupString += fmt.Sprintf("Set the region to %s. ", *input.Setup.Region)
		}

		return r.HandleMessageForEvaluation(setupString + input.Message)
	})
}

// HandleMessageForEvaluation processes a message and returns the response for evaluation purposes
func (r *Runner) HandleMessageForEvaluation(msg string) (string, error) {
	resp, err := genkit.Generate(r.ctx, r.g,
		ai.WithPrompt(msg),
		ai.WithTools(r.tools...),
		ai.WithReturnToolRequests(true),
	)
	if err != nil {
		return "", fmt.Errorf("Generate error: %w", err)
	}

	toolsCalled := []string{}
	for _, req := range resp.ToolRequests() {
		toolsCalled = append(toolsCalled, strings.ToLower(req.Name))
	}

	return fmt.Sprintf("Tools[%s]", strings.Join(toolsCalled, ", ")), nil
}
