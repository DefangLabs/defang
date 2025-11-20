package agent

import (
	"context"
	"encoding/json"

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type GenkitGenerator interface {
	Generate(ctx context.Context, prompt string, tools []ai.ToolRef, messages []*ai.Message, streamingCallback func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error)
}

type genkitGenerator struct {
	genkit *genkit.Genkit
}

func (g *genkitGenerator) Generate(ctx context.Context, prompt string, tools []ai.ToolRef, messages []*ai.Message, streamingCallback func(context.Context, *ai.ModelResponseChunk) error) (*ai.ModelResponse, error) {
	return genkit.Generate(ctx, g.genkit,
		ai.WithSystem(prompt),
		ai.WithTools(tools...),
		ai.WithMessages(messages...),
		ai.WithStreaming(streamingCallback),
		ai.WithReturnToolRequests(true),
	)
}

type Generator struct {
	genkitGenerator GenkitGenerator
	printer         Printer
	toolManager     *ToolManager
}

func NewGenerator(genkit *genkit.Genkit, printer Printer, toolManager *ToolManager) *Generator {
	return &Generator{
		genkitGenerator: &genkitGenerator{genkit: genkit},
		printer:         printer,
		toolManager:     toolManager,
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

func (g *Generator) GenerateLoop(ctx context.Context, prompt string, messages []*ai.Message, maxTurns int) ([]*ai.Message, error) {
	responseMessages := make([]*ai.Message, 0)
	for range maxTurns {
		resp, err := g.generate(ctx, prompt, messages)
		if err != nil {
			term.Debugf("error: %v", err)
			continue
		}

		responseMessages = append(responseMessages, resp.Message)

		toolRequests := resp.ToolRequests()
		if len(toolRequests) == 0 {
			return responseMessages, nil
		}

		if err := g.toolManager.equalPrevious(toolRequests); err != nil {
			return nil, err
		}

		toolResp := g.toolManager.HandleToolCalls(ctx, toolRequests)
		responseMessages = append(responseMessages, toolResp)
	}

	return responseMessages, nil
}

func (g *Generator) generate(ctx context.Context, prompt string, messages []*ai.Message) (*ai.ModelResponse, error) {
	g.printer.Println("* Thinking...")
	resp, err := g.genkitGenerator.Generate(
		ctx,
		prompt,
		g.toolManager.tools,
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
	for _, part := range resp.Message.Content {
		if part.Kind == ai.PartToolRequest {
			req := part.ToolRequest
			inputs, err := json.Marshal(req.Input)
			if err != nil {
				g.printer.Printf("! error marshaling tool request input: %v\n", err)
			} else {
				g.printer.Printf("* %s(%s)\n", req.Name, inputs)
			}
		}
	}

	return resp, nil
}

type EmptyResponseError struct{}

func (e *EmptyResponseError) Error() string {
	return "empty response from model"
}
