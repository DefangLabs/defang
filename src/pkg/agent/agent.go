package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/plugins/fabric"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/openai/openai-go/option"
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

var whitespacePattern = regexp.MustCompile(`^\s*$`)

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
		toolRef := ai.ToolRef(t)
		toolRefs[i] = toolRef
		action, ok := toolRef.(ai.Tool)
		if !ok {
			panic("toolRef is not an ai.Tool")
		}
		genkit.RegisterAction(g, action)
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

		// if input is empty or all whitespace, continue
		if whitespacePattern.MatchString(input) {
			continue
		}

		if err := a.handleMessage(input); err != nil {
			log.Printf("Error handling message: %v", err)
		}
	}
}

func (a *Agent) handleToolRequest(req *ai.ToolRequest) (*ai.ToolResponse, error) {
	inputs, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("error marshaling tool request input: %w", err)
	}
	term.Printf("* %s(%s)\n", req.Name, inputs)
	tool := genkit.LookupTool(a.g, req.Name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found", req.Name)
	}

	output, err := tool.RunRaw(a.ctx, req.Input)
	if err != nil {
		if errors.Is(err, common.ErrNoProviderSet) {
			return &ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: "Please set up a provider using one of the setup tools.",
			}, nil
		}
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

	parts := []*ai.Part{}
	for _, req := range requests {
		toolResp, err := a.handleToolRequest(req)
		if err != nil {
			return nil, fmt.Errorf("tool request error: %w", err)
		}
		_, _ = term.Printf("  > %s\n", toolResp.Output)
		parts = append(parts, ai.NewToolResponsePart(toolResp))
	}

	responses := []*ai.Message{ai.NewMessage(ai.RoleTool, nil, parts...)}
	a.msgs = append(a.msgs, responses...)
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithTools(a.tools...),
		ai.WithMessages(a.msgs...),
		ai.WithStreaming(a.streamingCallback),
	)
	if err != nil {
		return nil, fmt.Errorf("generation error: %w", err)
	}
	term.Println("")
	responses = append(responses, resp.Message)
	a.msgs = responses
	return responses, nil
}

func (a *Agent) streamingCallback(ctx context.Context, chunk *ai.ModelResponseChunk) error {
	for _, part := range chunk.Content {
		term.Print(part.Text)
	}
	return nil
}

func (a *Agent) handleMessage(msg string) error {
	a.msgs = append(a.msgs, ai.NewUserMessage(ai.NewTextPart(msg)))

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
		ai.WithReturnToolRequests(true),
		ai.WithStreaming(a.streamingCallback),
	)
	term.Print("\n")
	if err != nil {
		return fmt.Errorf("generation error: %w", err)
	}

	a.msgs = append(a.msgs, resp.Message)
	for _, part := range resp.Message.Content {
		term.Print(part.Text)
	}
	term.Println("")

	if len(resp.ToolRequests()) > 0 {
		_, err := a.handleToolCalls(resp.ToolRequests())
		if err != nil {
			return fmt.Errorf("tool call handling error: %w", err)
		}
	}

	return nil
}
