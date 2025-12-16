package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/agent/plugins/fabric"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core/api"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
	"github.com/openai/openai-go/option"
)

var whitespacePattern = regexp.MustCompile(`^\s*$`)

type Agent struct {
	generator *Generator
	printer   Printer
	system    string
}

func New(ctx context.Context, clusterAddr string, stack *stacks.StackParameters) (*Agent, error) {
	accessToken := cluster.GetExistingToken(clusterAddr)
	aiProvider := "fabric"
	var providerPlugin api.Plugin
	_, addr := cluster.SplitTenantHost(clusterAddr)
	// Generate a random session ID prepended with timestamp for easier sorting
	sessionID := fmt.Sprintf("%s-%s", time.Now().Format("20060102T150405Z"), pkg.RandomID())
	providerPlugin = &fabric.OpenAI{
		APIKey: accessToken,
		Opts: []option.RequestOption{
			option.WithBaseURL(fmt.Sprintf("https://%s/api/v1", addr)),
			option.WithHeader("x-defang-llm-session-id", sessionID),
		},
	}
	defaultModel := "google/gemini-2.5-flash"

	if os.Getenv("GOOGLE_API_KEY") != "" {
		aiProvider = "googleai"
		providerPlugin = &googlegenai.GoogleAI{}
		defaultModel = "gemini-2.5-flash"
	}

	model := pkg.Getenv("DEFANG_MODEL_ID", defaultModel)

	gk := genkit.Init(ctx,
		genkit.WithDefaultModel(fmt.Sprintf("%s/%s", aiProvider, model)),
		genkit.WithPlugins(providerPlugin),
	)

	elicitationsClient := elicitations.NewSurveyClient(os.Stdin, os.Stdout, os.Stderr)
	ec := elicitations.NewController(elicitationsClient)

	printer := printer{outStream: os.Stdout}
	toolManager := NewToolManager(gk, printer)
	defangTools := tools.CollectDefangTools(ec, tools.StackConfig{
		Cluster: clusterAddr,
		Stack:   stack,
	})
	toolManager.RegisterTools(defangTools...)
	fsTools := CollectFsTools()
	toolManager.RegisterTools(fsTools...)

	generator := NewGenerator(
		gk,
		printer,
		toolManager,
	)

	preparedSystemPrompt, err := prepareSystemPrompt()
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
	// The userPrompt is for the user only. Start the session with an empty message for the agent.
	return a.startSession(ctx, "")
}

func (a *Agent) StartWithMessage(ctx context.Context, msg string) error {
	return a.startSession(ctx, msg)
}

func (a *Agent) startSession(ctx context.Context, initialMessage string) error {
	signal.Reset(os.Interrupt) // unsubscribe the top-level signal handler

	a.printer.Printf("Type '/exit' to quit.\n")

	if initialMessage != "" {
		if err := a.handleUserMessage(ctx, initialMessage); err != nil {
			return fmt.Errorf("error handling initial message: %w", err)
		}
	}

	for {
		var input string
		err := survey.AskOne(
			&survey.Input{Message: ""},
			&input,
			survey.WithStdio(term.DefaultTerm.Stdio()),
			survey.WithIcons(func(icons *survey.IconSet) {
				icons.Question.Text = ">"
			}),
		)
		if err != nil {
			if errors.Is(err, terminal.InterruptErr) {
				return nil
			}
			return fmt.Errorf("error reading input: %w", err)
		}

		// if input is empty or all whitespace, continue
		if whitespacePattern.MatchString(input) {
			continue
		}

		if input == "/exit" {
			return nil
		}

		if err := a.handleUserMessage(ctx, input); err != nil {
			if errors.Is(err, context.Canceled) {
				continue
			}
			a.printer.Println("Error handling message:", err)
		}
	}
}

func (a *Agent) handleUserMessage(ctx context.Context, msg string) error {
	// Handle Ctrl+C during message handling / tool calls
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	const maxTurns = 8
	for {
		err := a.generator.HandleMessage(ctx, a.system, maxTurns, ai.NewUserMessage(ai.NewTextPart(msg)))
		if err == nil {
			return nil
		}
		if _, ok := err.(*maxTurnsReachedError); !ok {
			return err
		}

		var continueSession bool
		err = survey.AskOne(&survey.Confirm{
			Message: "Defang is still working on this, would you like to continue?",
			Default: true,
		}, &continueSession, survey.WithStdio(term.DefaultTerm.Stdio()))
		if err != nil {
			return fmt.Errorf("error prompting to continue session: %w", err)
		}
		if continueSession {
			continue
		}
		return nil
	}
}

func prepareSystemPrompt() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("error getting current working directory: %w", err)
	}
	currentDate := time.Now().Format(time.RFC3339)
	return fmt.Sprintf(
		"The current working directory is %q\nThe current date is %s",
		cwd,
		currentDate,
	), nil
}
