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
	ctx       context.Context
	g         *genkit.Genkit
	msgs      []*ai.Message
	prompt    string
	outStream io.Writer
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

	return &Agent{
		ctx:       ctx,
		g:         g,
		msgs:      []*ai.Message{},
		prompt:    prompt,
		outStream: os.Stdout,
	}
}

func (a *Agent) Printf(format string, args ...interface{}) {
	fmt.Fprintf(a.outStream, format, args...)
}

func (a *Agent) Println(args ...interface{}) {
	fmt.Fprintln(a.outStream, args...)
}

func (a *Agent) Start() error {
	reader := NewInputReader()
	defer reader.Close()

	a.Printf("\nWelcome to Defang. I can help you deploy your project to the cloud.\n")
	a.Printf("Type '/exit' to quit.\n")

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

func (a *Agent) streamingCallback(ctx context.Context, chunk *ai.ModelResponseChunk) error {
	for _, part := range chunk.Content {
		a.Printf("%s", part.Text)
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

	a.Printf("* Thinking...\r* ")

	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(prompt),
		ai.WithMessages(a.msgs...),
		ai.WithStreaming(a.streamingCallback),
	)
	a.Println("")
	if err != nil {
		return fmt.Errorf("generation error: %w", err)
	}

	a.msgs = append(a.msgs, resp.Message)
	for _, part := range resp.Message.Content {
		a.Printf("%s", part.Text)
	}
	a.Println("")

	return nil
}
