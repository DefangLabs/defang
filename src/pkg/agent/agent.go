package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/evaluators"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type AgentConfig struct {
	Cluster        string
	AuthPort       int
	EvaluationMode bool
	AIModel        string
	ProviderId     *client.ProviderID
	EvalMetrics    []evaluators.MetricConfig
}

type Agent struct {
	ctx            context.Context
	g              *genkit.Genkit
	tools          []ai.ToolRef
	evaluationMode bool
}

func New(ctx context.Context, cluster string, authPort int, providerId *client.ProviderID) *Agent {
	agentConfig := AgentConfig{
		Cluster:        cluster,
		AuthPort:       authPort,
		ProviderId:     providerId,
		EvaluationMode: true,
		EvalMetrics: []evaluators.MetricConfig{
			{MetricType: evaluators.EvaluatorRegex},
			{MetricType: evaluators.EvaluatorDeepEqual},
		},
	}

	tools := CollectTools(agentConfig.Cluster, agentConfig.AuthPort, agentConfig.ProviderId)
	return NewWithEvaluation(ctx, agentConfig, tools)
}

func NewWithEvaluation(ctx context.Context, config AgentConfig, tools []ai.Tool) *Agent {
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
	return &Agent{
		ctx:            ctx,
		g:              g,
		tools:          toolRefs,
		evaluationMode: config.EvaluationMode,
	}
}

// setup used for evaluation input structure
type DefangCLISetup struct {
	WorkingDirectory *string `json:"working_directory,omitempty"`
	Provider         *string `json:"provider,omitempty"`
	Region           *string `json:"region,omitempty"`
}
type DefangCLIInput struct {
	Message string         `json:"message"`
	Setup   DefangCLISetup `json:"setup,omitempty"`
}

// CreateEvaluationFlow creates a Genkit flow for evaluation purposes
func (a *Agent) CreateEvaluationFlow() *core.Flow[DefangCLIInput, string, struct{}] {
	return genkit.DefineFlow(a.g, "defang-cli", func(ctx context.Context, input DefangCLIInput) (string, error) {
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

		return a.HandleMessageForEvaluation(setupString + input.Message)
	})
}

func (a *Agent) Start() error {
	reader := NewInputReader()
	defer reader.Close()

	term.Println("\nWelcome to Defang. I can help you deploy your project to the cloud.")
	term.Println("Type '/exit' to quit.")

	for {
		term.Print("> ")

		input, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				term.Println("\nReceived termination signal, shutting down...")
				return nil
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("error reading input: %w", err)
		}

		if input == "/exit" {
			return nil
		}

		if err := a.handleMessage(input); err != nil {
			log.Printf("Error handling message: %v", err)
		}
	}
}

// GetGenkit returns the underlying Genkit instance for evaluation framework integration
func (a *Agent) GetGenkit() *genkit.Genkit {
	return a.g
}

// GetTools returns the available tools for evaluation
func (a *Agent) GetTools() []ai.ToolRef {
	return a.tools
}

// IsEvaluationMode returns whether the agent is in evaluation mode
func (a *Agent) IsEvaluationMode() bool {
	return a.evaluationMode
}

func (a *Agent) handleMessage(msg string) error {
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithMessages(a.msgs...),
	)
	if err != nil {
		return nil, fmt.Errorf("generation error: %w", err)
	}
	responses = append(responses, resp.Message)
	a.msgs = responses
	return responses, nil
}

func (a *Agent) handleMessage(msg string) error {
	a.msgs = append(a.msgs, ai.NewUserMessage(ai.NewTextPart(msg)))

	modelMessage := ai.NewMessage(ai.RoleModel, nil)
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	prompt := fmt.Sprintf("%s\n\nThe current working directory is %q", DefaultSystemPrompt, cwd)

	term.Print("* Thinking...\r* ")
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(prompt),
		ai.WithTools(a.tools...),
		ai.WithMessages(a.msgs...),
		ai.WithStreaming(func(ctx context.Context, chunk *ai.ModelResponseChunk) error {
			for _, part := range chunk.Content {
				term.Print(part.Text)
				modelMessage.Content = append(modelMessage.Content, part)
			}
			return nil
		}),
	)
	term.Print("\n")
	if err != nil {
		return fmt.Errorf("generation error: %w", err)
	}

	a.msgs = append(a.msgs, resp.Message)
	responses, err := a.handleToolCalls(resp.ToolRequests())
	if err != nil {
		return fmt.Errorf("tool call handling error: %w", err)
	}

	for _, msg := range responses {
		if msg.Role == ai.RoleTool {
			continue
		}
		for _, part := range msg.Content {
			if part.Kind == ai.PartText {
				term.Println(part.Text)
			}
		}
	}

	return nil
}
