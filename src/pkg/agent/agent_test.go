package agent

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock implementations for testing (simplified)
type mockPrinter struct {
	output []string
}

func (m *mockPrinter) Printf(format string, args ...interface{}) {
	m.output = append(m.output, fmt.Sprintf(format, args...))
}

func (m *mockPrinter) Println(args ...interface{}) {
	m.output = append(m.output, fmt.Sprintln(args...))
}

func TestPrepareSystemPrompt(t *testing.T) {
	originalPrompt := "You are a helpful assistant"

	result, err := prepareSystemPrompt(originalPrompt)

	assert.NoError(t, err)
	assert.Contains(t, result, originalPrompt)
	assert.Contains(t, result, "The current working directory is")

	// Check that it includes current working directory
	cwd, _ := os.Getwd()
	assert.Contains(t, result, cwd)
}
