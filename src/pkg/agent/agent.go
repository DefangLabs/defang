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
	"github.com/DefangLabs/defang/src/pkg/term"
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

If a deployment fails, use the 'logs' tool with the deployment_id to identify
issues and proactively advise the user on how to fix them.
`

var whitespacePattern = regexp.MustCompile(`^\s*$`)

type Agent struct {
	ctx       context.Context
	g         *genkit.Genkit
	maxTurns  int
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
		maxTurns:  3,
		msgs:      []*ai.Message{},
		prompt:    fmt.Sprintf("%s\n\nThe current working directory is %q", prompt, cwd),
		outStream: os.Stdout,
	}

	defangTools := CollectDefangTools(addr, providerId)
	fsTools := CollectFsTools()
	a.registerTools(defangTools...)
	a.registerTools(fsTools...)
	return a, nil
}

func (a *Agent) registerTools(tools ...ai.Tool) {
	for _, t := range tools {
		genkit.RegisterAction(a.g, t)
		a.tools = append(a.tools, ai.ToolRef(t))
	}
}

func (a *Agent) printf(format string, args ...interface{}) {
	fmt.Fprintf(a.outStream, format, args...)
}

func (a *Agent) println(args ...interface{}) {
	fmt.Fprintln(a.outStream, args...)
}

func (a *Agent) StartWithUserPrompt(userPrompt string) error {
	a.printf("\n%s\n", userPrompt)
	a.printf("Type '/exit' to quit.\n")
	return a.startSession()
}

func (a *Agent) StartWithMessage(msg string) error {
	a.printf("Type '/exit' to quit.\n")

	if err := a.handleUserMessage(msg); err != nil {
		return fmt.Errorf("error handling initial message: %w", err)
	}

	return a.startSession()
}

func (a *Agent) startSession() error {
	reader := NewInputReader()
	defer reader.Close()

	for {
		a.printf("> ")

		input, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				a.printf("\nReceived termination signal, shutting down...\n")
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

		if err := a.handleUserMessage(input); err != nil {
			a.printf("Error handling message: %v", err)
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
		if errors.Is(err, common.ErrNoProviderSet) {
			return &ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: "Please set up a provider using one of the setup tools.",
			}, nil
		}
		return nil, err
	}

	return &ai.ToolResponse{
		Name:   req.Name,
		Ref:    req.Ref,
		Output: output,
	}, nil
}

func (a *Agent) handleToolCalls(requests []*ai.ToolRequest) *ai.Message {
	parts := []*ai.Part{}
	for _, req := range requests {
		var part *ai.Part
		toolResp, err := a.handleToolRequest(req)
		if err != nil {
			a.printf("! %v", err)
			part = ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: err.Error(),
			})
		} else {
			a.println("~ ", toolResp.Output)
			part = ai.NewToolResponsePart(toolResp)
		}
		parts = append(parts, part)
	}

	return ai.NewMessage(ai.RoleTool, nil, parts...)
}

func (a *Agent) streamingCallback(ctx context.Context, chunk *ai.ModelResponseChunk) error {
	for _, part := range chunk.Content {
		if part.Kind == ai.PartText {
			a.printf("%s", part.Text)
		}
		if part.Kind == ai.PartReasoning {
			a.printf("_%s_", part.Text)
		}
	}
	return nil
}

func (a *Agent) handleUserMessage(msg string) error {
	a.msgs = append(a.msgs, ai.NewUserMessage(ai.NewTextPart(msg)))

	return a.generateLoop()
}

func (a *Agent) generateLoop() error {
	a.println("* Thinking...")

	prevTurnToolRequestsJSON := make(map[string]bool)

	for range a.maxTurns {
		resp, err := a.generate()
		if err != nil {
			term.Debugf("error: %v", err)
			continue
		}

		a.msgs = append(a.msgs, resp.Message)
		toolRequests := resp.ToolRequests()
		if len(toolRequests) == 0 {
			return nil
		}

		newToolsRequestsJSON := make(map[string]bool)
		for _, req := range toolRequests {
			inputs, err := json.Marshal(req.Input)
			if err != nil {
				return fmt.Errorf("error marshaling tool request input: %w", err)
			}
			currJSON := fmt.Sprintf("%s:%s", req.Name, inputs)
			newToolsRequestsJSON[currJSON] = true
			if prevTurnToolRequestsJSON[currJSON] {
				return fmt.Errorf("tool request %q with inputs %s repeated from previous turn, aborting to prevent infinite loop", req.Name, inputs)
			}
		}

		prevTurnToolRequestsJSON = newToolsRequestsJSON

		toolResp := a.handleToolCalls(toolRequests)
		a.msgs = append(a.msgs, toolResp)
	}

	return nil
}

type EmptyResponseError struct{}

func (e *EmptyResponseError) Error() string {
	return "empty response from model"
}

func (a *Agent) generate() (*ai.ModelResponse, error) {
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithSystem(a.prompt),
		ai.WithTools(a.tools...),
		ai.WithMessages(a.msgs...),
		ai.WithReturnToolRequests(true),
		ai.WithStreaming(a.streamingCallback),
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Message.Content) == 0 {
		return nil, &EmptyResponseError{}
	}
	a.println("")
	for _, part := range resp.Message.Content {
		if part.Kind == ai.PartToolRequest {
			req := part.ToolRequest
			inputs, err := json.Marshal(req.Input)
			if err != nil {
				a.printf("! error marshaling tool request input: %v\n", err)
			} else {
				a.printf("* %s(%s)\n", req.Name, inputs)
			}
		}
	}

	return resp, nil
}
