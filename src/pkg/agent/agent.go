package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

const DefaultSystemPrompt = `You are a helpful assistant. Your job is to help
the user deploy and manage their cloud applications using Defang. Defang is a
tool that makes it easy to deploy Docker Compose projects to cloud providers
like AWS, GCP, and Digital Ocean. Be as succinct, direct, and clear as
possible.
Some tools ask for a working_directory. This should usually be set to the
current working directory (or ".") unless otherwise specified by the user.
Some tools ask for a project_name. This is optional, but useful when working
on a project that is not in the current working directory.
`

type Agent struct {
	ctx            context.Context
	g              *genkit.Genkit
	tools          []ai.ToolRef
	evaluationMode bool
}

func New(ctx context.Context, cluster string, authPort int, providerId *client.ProviderID) *Agent {
	return NewWithEvaluation(ctx, cluster, authPort, providerId, false)
}

func NewWithEvaluation(ctx context.Context, cluster string, authPort int, providerId *client.ProviderID, evaluationMode bool) *Agent {
	// Initialize Genkit with the Google AI plugin
	g := genkit.Init(ctx,
		genkit.WithDefaultModel(fmt.Sprintf("%s/%s", provider, model)),
		genkit.WithPlugins(providerPlugin),
	)

	tools := CollectTools(addr, authPort, providerId)
	toolRefs := make([]ai.ToolRef, len(tools))
	for i, t := range tools {
		toolRefs[i] = ai.ToolRef(t)
	}
	return &Agent{
		ctx:            ctx,
		g:              g,
		tools:          toolRefs,
		evaluationMode: evaluationMode,
	}
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

// HandleMessageForEvaluation processes a message and returns the response for evaluation purposes
func (a *Agent) HandleMessageForEvaluation(msg string) (string, error) {
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(msg),
		ai.WithTools(a.tools...),
	)
	if err != nil {
		return "", fmt.Errorf("generation error: %w", err)
	}

	parts := []*ai.Part{}
	for _, req := range resp.ToolRequests() {
		tool := genkit.LookupTool(a.g, req.Name)
		if tool == nil {
			log.Printf("tool %q not found", req.Name)
			continue
		}

		output, err := tool.RunRaw(a.ctx, req.Input)
		if err != nil {
			log.Printf("tool %q execution failed: %v", tool.Name(), err)
			// Continue with error response rather than failing completely in evaluation mode
			if a.evaluationMode {
				output = fmt.Sprintf("Error executing tool %s: %v", req.Name, err)
			} else {
				log.Fatalf("tool %q execution failed: %v", tool.Name(), err)
			}
		}

		parts = append(parts,
			ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: output,
			}))
	}

	if len(parts) > 0 {
		resp, err = genkit.Generate(a.ctx, a.g,
			ai.WithMessages(append(resp.History(), ai.NewMessage(ai.RoleTool, nil, parts...))...),
		)
		if err != nil {
			return "", fmt.Errorf("generation error: %w", err)
		}
	}

	// Extract text response from messages
	var response strings.Builder
	for _, msg := range resp.History() {
		if msg.Role == ai.RoleUser {
			continue
		}
		for _, part := range msg.Content {
			if part.Kind == ai.PartText {
				response.WriteString(part.Text)
				response.WriteString("\n")
			}
		}
	}

	return strings.TrimSpace(response.String()), nil
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
