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

const DefaultSystemPrompt = `You are an interactive CLI tool called Defang.
Your purpose is to assist me with deploying, managing, and troubleshooting
issues with my cloud application. Use the instructions below and the tools
available to assist me.

All of the tools available to you are oriented around my project's compose
files (compose.yaml by default, but other filenames are possible). When
the 'deploy' tool is used, these files are uploaded, along with my app's
build context, and a 'cd' process is started in my cloud account. This 'cd'
process is responsible for provisioning infrastructure, building container
images, and deploying services.

I am most likely to ask you to perform tasks related to deploying my
application. For example, I may ask you to answer questions about my deployed
services, or to troubleshoot issues underlying a failed deployment.

When investigating failed deployments, proactively use the 'logs' tool to
retrieve output for the relevant 'deployment_id'. If you have the times at
which the deployment started and ended, use the 'since' and 'until' parameter
to ensure you get the complete set of logs. If you only have 'since' or 'until'
note that logs will be returned in pages and there may be more logs in the next
page. After fetching the logs, it may be helpful to read the compose file and
any 'Dockerfile' which is mentioned explicitly or referenced implicitly. It may
also be helpful to read my app's source code and configuration files. The
compose file will describe how each service is built, configured, deployed and
run. Based on the errors you see in the logs, and your understanding of the
compose file and my app's source code, describe the issues you uncover, and
suggest potential fixes.

If you find information which suggests that there is no problem, assume that I
am right and you are mistaken. Continue investigating until you find a definite
issue. If you need more information to understand the issue, or to propose a fix,
ask me for more information.

After fetching the logs, summarize the relevant errors, warnings, and other
information, form a plan to understand the issue and propose a solution.
Explain your reasoning step-by-step. Use the tools available to you as needed.

Some tools ask for a 'working_directory'. This should usually be set to the
current working directory (or ".") unless otherwise specified by the user.
Some tools ask for a 'project_name'. This is usually optional, but it is useful
when performing tasks related to a project that is not the project in the
current working directory. For example, if you need to inspect or destroy a
previously deployed application.
Some tools ask for 'compose_file_paths', this is usually not needed if the
'working_directory' is provided, however, this should be used if the user has
specified specific compose files to use.

Be concise, direct, and to the point unless user asks for detail. Answer the
user's question directly, without elaboration, explanation, or details. Be
proactive when the user asks you to do something, but NEVER start a deployment
unless explicitly instructed to do so. Deployments may take a long time.
`

var whitespacePattern = regexp.MustCompile(`^\s*$`)

type Agent struct {
	generator *Generator
	printer   Printer
	system    string
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
	err := a.generator.HandleMessage(ctx, a.system, 8, ai.NewUserMessage(ai.NewTextPart(msg)))
	if err != nil {
		return err
	}

	return nil
}

func prepareSystemPrompt(prompt string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current working directory: %w", err)
	}
	return fmt.Sprintf("%s\n\nThe current working directory is %q", prompt, cwd), nil
}
