package agent

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type GenkitGenerator interface {
	Generate(ctx context.Context, prompt string, messages []*ai.Message, streamingCallback func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error)
}

type genkitGenerator struct {
	genkit *genkit.Genkit
}

func (g *genkitGenerator) Generate(ctx context.Context, prompt string, messages []*ai.Message, streamingCallback func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error) {
	return genkit.Generate(ctx, g.genkit,
		ai.WithSystem(prompt),
		ai.WithMessages(messages...),
		ai.WithStreaming(streamingCallback),
		ai.WithReturnToolRequests(true),
	)
}

type Generator struct {
	messages        []*ai.Message
	genkitGenerator GenkitGenerator
	printer         Printer
}

func NewGenerator(genkit *genkit.Genkit, printer Printer) *Generator {
	return &Generator{
		genkitGenerator: &genkitGenerator{genkit: genkit},
		printer:         printer,
	}
}

func (g *Generator) streamingCallback(_ context.Context, chunk *ai.ModelResponseChunk) error {
	for _, part := range chunk.Content {
		if part.Kind == ai.PartText {
			g.printer.Printf("%s", part.Text)
		}
		if part.Kind == ai.PartReasoning {
			g.printer.Printf("_%s_", part.Text)
		}
	}
	return nil
}

func (g *Generator) HandleMessage(ctx context.Context, prompt string, message *ai.Message) {
	if message != nil {
		g.messages = append(g.messages, message)
	}
	resp, err := g.generate(ctx, prompt, g.messages)
	if err != nil {
		term.Debugf("error: %v", err)
	}

	g.messages = append(g.messages, resp.Message)
}

func (g *Generator) generate(ctx context.Context, prompt string, messages []*ai.Message) (*ai.ModelResponse, error) {
	g.printer.Println("* Thinking...")
	resp, err := g.genkitGenerator.Generate(
		ctx,
		prompt,
		messages,
		g.streamingCallback,
	)
	if err != nil {
		return nil, err
	}
	if len(resp.Message.Content) == 0 {
		return nil, &EmptyResponseError{}
	}
	g.printer.Println("")

	return resp, nil
}

type EmptyResponseError struct{}

func (e *EmptyResponseError) Error() string {
	return "empty response from model"
}
