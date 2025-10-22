package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/plugins/fabric"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/openai/openai-go/option"
)

const DefaultSystemPrompt = "You are a helpful assistant. Your job is to help the user deploy and manage their cloud applications using Defang. Defang is a tool that makes it easy to deploy Docker Compose projects to cloud providers like AWS, GCP, and Digital Ocean."

type Agent struct {
	ctx    context.Context
	g      *genkit.Genkit
	msgs   []*ai.Message
	prompt string
	tools  []ai.ToolRef
}

func New(ctx context.Context, addr string, providerId *client.ProviderID, prompt string) *Agent {
	accessToken := cluster.GetExistingToken(addr)
	provider := "fabric"
	var providerPlugin api.Plugin
	providerPlugin = &fabric.OpenAI{
		APIKey: accessToken,
		Opts: []option.RequestOption{
			option.WithBaseURL(fmt.Sprintf("https://%s/api/v1", addr)),
		},
	}
	defaultModel := "google/gemini-2.5-flash"

	if os.Getenv("GOOGLE_API_KEY") != "" {
		provider = "googleai"
		providerPlugin = &googlegenai.GoogleAI{}
		defaultModel = "gemini-2.5-flash"
	}

	model := pkg.Getenv("DEFANG_MODEL_ID", defaultModel)

	g := genkit.Init(ctx,
		genkit.WithDefaultModel(fmt.Sprintf("%s/%s", provider, model)),
		genkit.WithPlugins(providerPlugin),
	)

	tools := CollectTools(addr, providerId)
	toolRefs := make([]ai.ToolRef, len(tools))
	for i, t := range tools {
		toolRefs[i] = ai.ToolRef(t)
	}
	return &Agent{
		ctx:    ctx,
		g:      g,
		msgs:   []*ai.Message{},
		prompt: prompt,
		tools:  toolRefs,
	}
}

func (a *Agent) Start() error {
	reader := NewInputReader()
	defer reader.Close()

	fmt.Println("Type 'exit' to quit.")

	for {
		fmt.Print("> ")

		input, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				fmt.Println("\nReceived termination signal, shutting down...")
				return nil
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("error reading input: %w", err)
		}

		if input == "exit" {
			return nil
		}

		if err := a.handleMessage(input); err != nil {
			log.Printf("Error handling message: %v", err)
		}
	}
}

func (a *Agent) handleToolRequest(req *ai.ToolRequest) (*ai.ToolResponse, error) {
	tool := genkit.LookupTool(a.g, req.Name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found", req.Name)
	}

	output, err := tool.RunRaw(a.ctx, req.Input)
	if err != nil {
		return nil, fmt.Errorf("tool %q execution error: %w", tool.Name(), err)
	}

	return &ai.ToolResponse{
		Name:   req.Name,
		Ref:    req.Ref,
		Output: output,
	}, nil
}

func (a *Agent) handleToolCalls(requests []*ai.ToolRequest) ([]*ai.Message, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	var responses []*ai.Message
	parts := []*ai.Part{}
	for _, req := range requests {
		toolResp, err := a.handleToolRequest(req)
		if err != nil {
			return nil, fmt.Errorf("tool request error: %w", err)
		}
		parts = append(parts, ai.NewToolResponsePart(toolResp))
	}

	responses = append(responses, ai.NewMessage(ai.RoleTool, nil, parts...))
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

	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(a.prompt),
		ai.WithTools(a.tools...),
		ai.WithMessages(a.msgs...),
		ai.WithStreaming(func(ctx context.Context, chunk *ai.ModelResponseChunk) error {
			for _, part := range chunk.Content {
				fmt.Print(part.Text)
				modelMessage.Content = append(modelMessage.Content, part)
			}
			return nil
		}),
	)
	fmt.Print("\n")
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
				fmt.Println(part.Text)
			}
		}
	}

	return nil
}
