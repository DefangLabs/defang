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
	result, err := prepareSystemPrompt()

	assert.NoError(t, err)
	assert.Contains(t, result, "The current working directory is")
	assert.Contains(t, result, "The current date is")

	// Check that it includes current working directory
	cwd, _ := os.Getwd()
	assert.Contains(t, result, cwd)
}

func TestSelectModel(t *testing.T) {
	tests := []struct {
		name         string
		googleAPIKey string
		modelID      string
		wantProvider string
		wantModel    string
	}{
		{name: "fabric default", wantProvider: "fabric", wantModel: "default"},
		{name: "fabric override", modelID: "google/gemini-3.5-flash", wantProvider: "fabric", wantModel: "google/gemini-3.5-flash"},
		{name: "google api key", googleAPIKey: "key", wantProvider: "googleai", wantModel: "gemini-3.5-flash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setOrUnsetenv(t, "GOOGLE_API_KEY", tt.googleAPIKey)
			setOrUnsetenv(t, "DEFANG_MODEL_ID", tt.modelID)
			provider, model := selectModel()
			if provider != tt.wantProvider || model != tt.wantModel {
				t.Errorf("selectModel() = %q, %q; want %q, %q", provider, model, tt.wantProvider, tt.wantModel)
			}
		})
	}
}

func setOrUnsetenv(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value) // restores the original value after the test
	if value == "" {
		os.Unsetenv(key)
	}
}
