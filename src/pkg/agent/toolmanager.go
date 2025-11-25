package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/agent/tools"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
)

type GenkitToolManager interface {
	RegisterTools(tools ...ai.Tool)
	LookupTool(name string) ai.Tool
}

type genkitToolManager struct {
	genkit *genkit.Genkit
}

func (g *genkitToolManager) RegisterTools(tools ...ai.Tool) {
	for _, tool := range tools {
		genkit.RegisterAction(g.genkit, tool)
	}
}

func (g *genkitToolManager) LookupTool(name string) ai.Tool {
	return genkit.LookupTool(g.genkit, name)
}

func NewGenkitToolManager(genkit *genkit.Genkit) GenkitToolManager {
	return &genkitToolManager{genkit: genkit}
}

type ToolManager struct {
	gktm                     GenkitToolManager
	printer                  Printer
	prevTurnToolRequestsJSON map[string]bool
	tools                    []ai.ToolRef
}

func NewToolManager(genkit *genkit.Genkit, printer Printer) *ToolManager {
	return &ToolManager{
		gktm:                     NewGenkitToolManager(genkit),
		printer:                  printer,
		prevTurnToolRequestsJSON: make(map[string]bool),
		tools:                    make([]ai.ToolRef, 0),
	}
}

func (t *ToolManager) RegisterTools(tools ...ai.Tool) {
	for _, tool := range tools {
		t.gktm.RegisterTools(tool)
		t.tools = append(t.tools, ai.ToolRef(tool))
	}
}

func (t *ToolManager) HandleToolCalls(ctx context.Context, requests []*ai.ToolRequest) *ai.Message {
	if t.EqualPrevious(requests) {
		return ai.NewMessage(ai.RoleTool, nil, ai.NewToolResponsePart(&ai.ToolResponse{
			Name:   "error",
			Ref:    "error",
			Output: "The same tool request was made in the previous turn. To prevent infinite loops, no action was taken.",
		}))
	}

	parts := []*ai.Part{}
	for _, req := range requests {
		var part *ai.Part
		toolResp, err := t.handleToolRequest(ctx, req)
		if err != nil {
			t.printer.Printf("! %v", err)
			part = ai.NewToolResponsePart(&ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: err.Error(),
			})
		} else {
			t.printer.Println("~ ", toolResp.Output)
			part = ai.NewToolResponsePart(toolResp)
		}
		parts = append(parts, part)
	}

	return ai.NewMessage(ai.RoleTool, nil, parts...)
}

func (t *ToolManager) handleToolRequest(ctx context.Context, req *ai.ToolRequest) (*ai.ToolResponse, error) {
	tool := t.gktm.LookupTool(req.Name)
	if tool == nil {
		return nil, fmt.Errorf("tool %q not found", req.Name)
	}

	output, err := tools.TeeTerm(func() (any, error) {
		return tool.RunRaw(ctx, req.Input)
	})
	if err != nil {
		if errors.Is(err, common.ErrNoProviderSet) {
			return &ai.ToolResponse{
				Name:   req.Name,
				Ref:    req.Ref,
				Output: "Please set up a provider using one of the setup tools.",
			}, nil
		}
		return nil, err
	}

	return &ai.ToolResponse{
		Name:   req.Name,
		Ref:    req.Ref,
		Output: output,
	}, nil
}

func (t *ToolManager) EqualPrevious(toolRequests []*ai.ToolRequest) bool {
	newToolsRequestsJSON := make(map[string]bool)
	for _, req := range toolRequests {
		inputs, err := json.Marshal(req.Input)
		if err != nil {
			term.Debugf("error marshaling tool request input: %v", err)
		}
		currJSON := fmt.Sprintf("%s:%s", req.Name, inputs)
		newToolsRequestsJSON[currJSON] = true
		if t.prevTurnToolRequestsJSON[currJSON] {
			return true
		}
	}

	t.prevTurnToolRequestsJSON = newToolsRequestsJSON
	return false
}
