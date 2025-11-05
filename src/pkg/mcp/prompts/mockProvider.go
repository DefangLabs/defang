package prompts

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

type MockProvider struct {
	client.Provider
	// Add fields if needed for test state
}

func (f *MockProvider) ID() client.ProviderID                                        { return client.ProviderGCP }
func (f *MockProvider) AccountInfo(ctx context.Context) (*client.AccountInfo, error) { return nil, nil }

// Add stubs for all other methods if needed for compilation
