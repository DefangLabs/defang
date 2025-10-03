package agent

import (
	"bufio"
	"context"
	"errors"
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

func New(ctx context.Context) *Agent {
	// Initialize Genkit with the Google AI plugin
	g := genkit.Init(ctx,
		genkit.WithPlugins(&googlegenai.GoogleAI{}),
		genkit.WithDefaultModel("googleai/gemini-2.5-flash"),
	)
	// Define the weather tool
	weatherTool := genkit.DefineTool(
		g,
		"getWeather",
		"Gets the current weather for a location",
		func(ctx *ai.ToolContext, input *WeatherInput) (string, error) {
			// Simulated weather data
			return fmt.Sprintf("The weather in %s is sunny and 72°F", input.Location), nil
		},
	)

	// Define the calculator tool
	calculatorTool := genkit.DefineTool(
		g,
		"calculator",
		"Performs basic arithmetic operations (add, subtract, multiply, divide)",
		func(ctx *ai.ToolContext, input *CalculatorInput) (float64, error) {
			switch input.Operation {
			case "add":
				return input.A + input.B, nil
			case "subtract":
				return input.A - input.B, nil
			case "multiply":
				return input.A * input.B, nil
			case "divide":
				if input.B == 0 {
					return 0, errors.New("division by zero")
				}
				return input.A / input.B, nil
			default:
				return 0, fmt.Errorf("invalid operation: %s", input.Operation)
			}
		},
	)

	return &Agent{
		ctx:   ctx,
		g:     g,
		tools: []ai.ToolRef{weatherTool, calculatorTool},
	}
}

// Define input/output types for your tools
type WeatherInput struct {
	Location string `json:"location"`
}

type CalculatorInput struct {
	Operation string  `json:"operation"`
	A         float64 `json:"a"`
	B         float64 `json:"b"`
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
