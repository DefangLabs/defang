package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/plugins/fabric"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/openai/openai-go/option"
)

const DefaultSystemPrompt = `You are an interactive CLI tool that helps users
tasks related to their cloud application deployments. Use the instructions
below and the tools available to you to assist the user.

Be concise, direct, and to the point. You MUST answer concisely with fewer than
4 lines (not including tool use or code generation), unless user asks for
detail. Answer the user's question directly, without elaboration, explanation,
or details. One word answers are best.

Be proactive when the user asks you to do something, but NEVER start a
deployment unless explicitly instructed to do so. Deployments may take a long
time.

The user will primarily request you perform tasks related to deploying their
application to cloud providers and answering questions about their deployed
services, and understanding issues underlying failed deployments.

Application's are deployed to the cloud with Defang. Defang is a CLI tool
that allows users to describe their application using Docker Compose files.
These files are typically called 'compose.yaml', 'docker-compose.yaml',
'docker-compose.yml', or 'compose.yml'. Defang interprets these files and
provisions cloud-specific resources to run the services described within.
Defang supports multiple cloud providers, including AWS and GCP.

Some tools ask for a working_directory. This should usually be set to the
current working directory (or ".") unless otherwise specified by the user.
Some tools ask for a project_name. This is optional, but useful when working
on a project that is not in the current working directory.
`

var whitespacePattern = regexp.MustCompile(`^\s*$`)

type Agent struct {
	ctx       context.Context
	g         *genkit.Genkit
	msgs      []*ai.Message
	prompt    string
	tools     []ai.ToolRef
	outStream io.Writer
}

func New(ctx context.Context, addr string, providerId *client.ProviderID, prompt string) (*Agent, error) {
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

	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("error getting current working directory: %w", err)
	}

	a := &Agent{
		ctx:       ctx,
		g:         g,
		msgs:      []*ai.Message{},
		prompt:    fmt.Sprintf("%s\n\nThe current working directory is %q", prompt, cwd),
		outStream: os.Stdout,
	}

	defangTools := CollectDefangTools(addr, providerId)
	fsTools := CollectFsTools()
	a.RegisterTools(defangTools...)
	a.RegisterTools(fsTools...)
	return a, nil
}

func (a *Agent) RegisterTools(tools ...ai.Tool) {
	for _, t := range tools {
		genkit.RegisterAction(a.g, t)
		a.tools = append(a.tools, ai.ToolRef(t))
	}
}

func (a *Agent) Printf(format string, args ...interface{}) {
	fmt.Fprintf(a.outStream, format, args...)
}

func (a *Agent) Println(args ...interface{}) {
	fmt.Fprintln(a.outStream, args...)
}

func (a *Agent) StartWithUserPrompt(userPrompt string) error {
	a.Printf("\n%s\n", userPrompt)
	a.Printf("Type '/exit' to quit.\n")
	return a.startSession()
}

func (a *Agent) StartWithMessage(msg string) error {
	a.Printf("Type '/exit' to quit.\n")

	if err := a.handleMessage(msg); err != nil {
		return fmt.Errorf("error handling initial message: %w", err)
	}

	return a.startSession()
}

func (a *Agent) startSession() error {
	reader := NewInputReader()
	defer reader.Close()

	for {
		a.Printf("> ")

		input, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				a.Printf("\nReceived termination signal, shutting down...\n")
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
			a.Printf("Error handling message: %v", err)
		}
	}
}

func (a *Agent) handleToolRequest(req *ai.ToolRequest) (*ai.ToolResponse, error) {
	inputs, err := json.Marshal(req.Input)
	if err != nil {
		return nil, fmt.Errorf("error marshaling tool request input: %w", err)
	}
	a.Printf("* %s(%s)\n", req.Name, inputs)
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
		a.Printf("  > %s\n", toolResp.Output)
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
	a.Println("")
	responses = append(responses, resp.Message)
	a.msgs = responses
	return responses, nil
}

func (a *Agent) streamingCallback(ctx context.Context, chunk *ai.ModelResponseChunk) error {
	for _, part := range chunk.Content {
		a.Printf("%s", part.Text)
	}
	return nil
}

func (a *Agent) handleMessage(msg string) error {
	a.msgs = append(a.msgs, ai.NewUserMessage(ai.NewTextPart(msg)))

	a.Printf("* Thinking...\r* ")

	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(a.prompt),
		ai.WithTools(a.tools...),
		ai.WithMessages(a.msgs...),
		ai.WithReturnToolRequests(true),
		ai.WithStreaming(a.streamingCallback),
	)
	a.Println("")
	if err != nil {
		return fmt.Errorf("generation error: %w", err)
	}

	a.msgs = append(a.msgs, resp.Message)

	if len(resp.ToolRequests()) > 0 {
		_, err := a.handleToolCalls(resp.ToolRequests())
		if err != nil {
			return fmt.Errorf("tool call handling error: %w", err)
		}
	}

	return nil
}
