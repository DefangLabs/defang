package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
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

var whitespacePattern = regexp.MustCompile(`^\s*$`)

type Agent struct {
	generator *Generator
	printer   Printer
	system    string
}

func New(ctx context.Context, clusterAddr string, providerId *client.ProviderID) (*Agent, error) {
	accessToken := cluster.GetExistingToken(clusterAddr)
	provider := "fabric"
	var providerPlugin api.Plugin
	_, addr := cluster.SplitTenantHost(clusterAddr)
	// Generate a random session ID prepended with timestamp for easier sorting
	sessionID := fmt.Sprintf("%s-%s", time.Now().Format("20060102T150405Z"), pkg.RandomID())
	providerPlugin = &fabric.OpenAI{
		APIKey: accessToken,
		Opts: []option.RequestOption{
			option.WithBaseURL(fmt.Sprintf("https://%s/api/v1", addr)),
			option.WithHeader("X-Defang-Agent-Session-Id", sessionID),
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

	generator := NewGenerator(
		gk,
		printer,
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
	for {
		a.generator.HandleMessage(ctx, a.system, ai.NewUserMessage(ai.NewTextPart(msg)))

		var continueSession bool
		err := survey.AskOne(&survey.Confirm{
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
