package agent

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/DefangLabs/defang/src/pkg/agent/plugins/gateway"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/openai/openai-go/option"
)

type Agent struct {
	ctx   context.Context
	g     *genkit.Genkit
	tools []ai.ToolRef
}

func New(ctx context.Context, cluster string, authPort int, providerId *client.ProviderID) *Agent {
	oai := &gateway.OpenAI{
		APIKey: "FAKE_TOKEN",
		Opts: []option.RequestOption{
			option.WithBaseURL("http://localhost:8080/api/v1"),
		},
	}

	g := genkit.Init(ctx,
		genkit.WithDefaultModel("gateway/publishers/google/models/gemini-2.5-flash"),
		genkit.WithPlugins(oai),
	)

	tools := CollectTools(cluster, authPort, providerId)
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
