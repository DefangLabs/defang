package agent

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

type Agent struct {
	ctx   context.Context
	g     *genkit.Genkit
	tools []ai.ToolRef
}

func New(ctx context.Context, cluster string, authPort int) *Agent {
	// Initialize Genkit with the Google AI plugin
	g := genkit.Init(ctx,
		genkit.WithPlugins(&googlegenai.GoogleAI{}),
		genkit.WithDefaultModel("googleai/gemini-2.5-flash"),
	)

	tools := CollectTools(cluster, authPort)
	toolRefs := make([]ai.ToolRef, len(tools))
	for i, t := range tools {
		toolRefs[i] = ai.ToolRef(t)
	}
	return &Agent{
		ctx:   ctx,
		g:     g,
		tools: toolRefs,
	}
}

func (a *Agent) Start() error {
	// prompt the user for input
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type 'exit' to quit.")
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if input == "exit" {
			break
		}

		err := a.handleMessage(input)
		if err != nil {
			log.Printf("Error handling message: %v", err)
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input: %w", err)
	}

	return nil
}

func (a *Agent) handleMessage(msg string) error {
	resp, err := genkit.Generate(a.ctx, a.g,
		ai.WithPrompt(msg),
		ai.WithTools(a.tools...),
		ai.WithReturnToolRequests(true),
	)
	if err != nil {
		return fmt.Errorf("generation error: %w", err)
	}

	parts := []*ai.Part{}
	for _, req := range resp.ToolRequests() {
		tool := genkit.LookupTool(a.g, req.Name)
		if tool == nil {
			log.Fatalf("tool %q not found", req.Name)
		}

		output, err := tool.RunRaw(a.ctx, req.Input)
		if err != nil {
			log.Fatalf("tool %q execution failed: %v", tool.Name(), err)
		}

		parts = append(parts,
			ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: output,
			}))
	}

	if len(parts) > 0 {
		resp, err = genkit.Generate(a.ctx, a.g,
			ai.WithMessages(append(resp.History(), ai.NewMessage(ai.RoleTool, nil, parts...))...),
		)
		if err != nil {
			return fmt.Errorf("generation error: %w", err)
		}
	}

	for _, msg := range resp.History() {
		for _, part := range msg.Content {
			if part.Kind == ai.PartText {
				fmt.Println(part.Text)
			}
		}
	}

	return nil
}
