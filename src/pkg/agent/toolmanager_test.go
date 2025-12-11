package agent

import (
	"testing"

	"github.com/firebase/genkit/go/ai"
)

func TestToolManager_EqualPrevious(t *testing.T) {
	tm := &ToolManager{
		prevTurnToolRequestsJSON: make(map[string]bool),
	}

	// Helper to create ToolRequest
	newReq := func(name string, input any) *ai.ToolRequest {
		return &ai.ToolRequest{Name: name, Input: input}
	}

	// First call, should return false (no previous)
	reqs1 := []*ai.ToolRequest{
		newReq("toolA", map[string]any{"foo": "bar"}),
		newReq("toolB", map[string]any{"baz": 42}),
	}
	if tm.EqualPrevious(reqs1) {
		t.Errorf("expected false on first call, got true")
	}

	// Second call, same requests, should return true (loop detected)
	if !tm.EqualPrevious(reqs1) {
		t.Errorf("expected true for identical requests, got false")
	}

	// Third call, different input, should return false
	reqs2 := []*ai.ToolRequest{
		newReq("toolA", map[string]any{"foo": "bar"}),
		newReq("toolB", map[string]any{"baz": 43}), // changed value
	}
	if tm.EqualPrevious(reqs2) {
		t.Errorf("expected false for different requests, got true")
	}

	// Fourth call, same as third, should return true
	if !tm.EqualPrevious(reqs2) {
		t.Errorf("expected true for identical requests, got false")
	}

	// Fifth call, different length, should return false
	reqs3 := []*ai.ToolRequest{
		newReq("toolA", map[string]any{"foo": "bar"}),
	}
	if tm.EqualPrevious(reqs3) {
		t.Errorf("expected false for different length, got true")
	}

	// Sixth call, same as fifth, should return true
	if !tm.EqualPrevious(reqs3) {
		t.Errorf("expected true for identical requests, got false")
	}
}
