package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

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
	generator *Generator
	printer   Printer
	system    string
	msgs      []*ai.Message
}

func New(ctx context.Context, addr string, providerId *client.ProviderID, system string) (*Agent, error) {
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

	gk := genkit.Init(ctx,
		genkit.WithDefaultModel(fmt.Sprintf("%s/%s", provider, model)),
		genkit.WithPlugins(providerPlugin),
	)

	printer := printer{outStream: os.Stdout}
	toolManager := NewToolManager(gk, printer)
	defangTools := CollectDefangTools(addr, providerId)
	toolManager.RegisterTools(defangTools...)
	fsTools := CollectFsTools()
	toolManager.RegisterTools(fsTools...)

	generator := NewGenerator(
		gk,
		printer,
		toolManager,
	)

	preparedSystemPrompt, err := prepareSystemPrompt(system)
	if err != nil {
		return nil, err
	}

	a := &Agent{
		printer:   printer,
		generator: generator,
		system:    preparedSystemPrompt,
		msgs:      make([]*ai.Message, 0),
	}

	return a, nil
}

func (a *Agent) StartWithUserPrompt(ctx context.Context, userPrompt string) error {
	a.printer.Printf("\n%s\n", userPrompt)
	a.printer.Printf("Type '/exit' to quit.\n")
	return a.startSession(ctx)
}

func (a *Agent) StartWithMessage(ctx context.Context, msg string) error {
	a.printer.Printf("Type '/exit' to quit.\n")

	if err := a.handleUserMessage(ctx, msg); err != nil {
		return fmt.Errorf("error handling initial message: %w", err)
	}

	return a.startSession(ctx)
}

func (a *Agent) startSession(ctx context.Context) error {
	reader := NewInputReader()
	defer reader.Close()

	for {
		a.printer.Printf("> ")

		input, err := reader.ReadLine()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				a.printer.Printf("\nReceived termination signal, shutting down...\n")
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

		if err := a.handleUserMessage(ctx, input); err != nil {
			a.printer.Println("Error handling message: %v", err)
		}
	}
}

func (a *Agent) handleUserMessage(ctx context.Context, msg string) error {
	a.msgs = append(a.msgs, ai.NewUserMessage(ai.NewTextPart(msg)))
	responseMessages, err := a.generator.GenerateLoop(ctx, a.system, a.msgs, 3)
	if err != nil {
		return err
	}

	a.msgs = append(a.msgs, responseMessages...)
	return nil
}

func prepareSystemPrompt(prompt string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current working directory: %w", err)
	}
	return fmt.Sprintf("%s\n\nThe current working directory is %q", prompt, cwd), nil
}
