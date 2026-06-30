package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCleanerProvider embeds MockProvider and adds the optional OrphanCleaner capability,
// recording which resources were cleaned up. A pointer is used so recording persists.
type mockCleanerProvider struct {
	client.MockProvider
	orphans     []client.OrphanResource
	discoverErr error
	cleanupErr  error
	cleaned     []string
}

func (m *mockCleanerProvider) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	return m.orphans, m.discoverErr
}

func (m *mockCleanerProvider) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	if m.cleanupErr != nil {
		return m.cleanupErr
	}
	m.cleaned = append(m.cleaned, r.ID)
	return nil
}

// MockCleanupCLI implements CLIInterface, returning a configurable provider.
type MockCleanupCLI struct {
	CLIInterface
	provider    client.Provider
	projectName string
}

func (m *MockCleanupCLI) Connect(ctx context.Context, fabricAddr string) (*client.GrpcClient, error) {
	return &client.GrpcClient{}, nil
}

func (m *MockCleanupCLI) InteractiveLoginMCP(ctx context.Context, fabricAddr string, mcpClient string) error {
	return nil
}

func (m *MockCleanupCLI) NewProvider(ctx context.Context, providerId client.ProviderID, grpcClient client.FabricClient, stack string) client.Provider {
	if m.provider != nil {
		return m.provider
	}
	return client.MockProvider{}
}

func (m *MockCleanupCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	return m.projectName, nil
}

func (m *MockCleanupCLI) CanIUseProvider(ctx context.Context, grpcClient *client.GrpcClient, provider client.Provider, projectName string, serviceCount int) error {
	return nil
}

func orphan(id, category string) client.OrphanResource {
	return client.OrphanResource{ID: id, Category: category, Name: id, Action: "do the thing"}
}

func TestHandleCleanupTool(t *testing.T) {
	loader := &client.MockLoader{}
	twoOrphans := []client.OrphanResource{orphan("alb:1", "alb"), orphan("ecr:repo", "ecr")}

	tests := []struct {
		name           string
		provider       client.Provider
		confirm        string
		notInteractive bool
		expectErr      string
		expectContains []string
		expectCleaned  []string
	}{
		{
			name:           "non-AWS provider is unsupported",
			provider:       client.MockProvider{},
			expectContains: []string{"only supported for AWS"},
		},
		{
			name:           "no orphans found",
			provider:       &mockCleanerProvider{},
			expectContains: []string{"No leftover resources"},
		},
		{
			name:           "confirm yes cleans every orphan",
			provider:       &mockCleanerProvider{orphans: twoOrphans},
			confirm:        "yes",
			expectContains: []string{"2 cleaned, 0 skipped, 0 failed", "defang down"},
			expectCleaned:  []string{"alb:1", "ecr:repo"},
		},
		{
			name:           "confirm no skips every orphan",
			provider:       &mockCleanerProvider{orphans: twoOrphans},
			confirm:        "no",
			expectContains: []string{"0 cleaned, 2 skipped, 0 failed"},
			expectCleaned:  nil,
		},
		{
			name:           "non-interactive reports only",
			provider:       &mockCleanerProvider{orphans: twoOrphans},
			notInteractive: true,
			expectContains: []string{"would do the thing", "interactive session"},
			expectCleaned:  nil,
		},
		{
			name:      "discovery error is surfaced",
			provider:  &mockCleanerProvider{discoverErr: errors.New("boom")},
			expectErr: "failed to discover leftover resources: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir("testdata")
			mockCLI := &MockCleanupCLI{provider: tt.provider, projectName: "test-project"}

			ec := elicitations.NewController(&mockElicitationsClient{
				responses: map[string]string{"confirm": tt.confirm},
			})
			if tt.notInteractive {
				ec.SetSupported(false)
			}

			stack := stacks.Parameters{Name: "test-stack", Provider: client.ProviderAWS}
			params := CleanupParams{LoaderParams: common.LoaderParams{WorkingDirectory: "."}}
			result, err := HandleCleanupTool(t.Context(), loader, params, mockCLI, ec, StackConfig{
				FabricAddr: "test-cluster",
				Stack:      &stack,
			})

			if tt.expectErr != "" {
				assert.EqualError(t, err, tt.expectErr)
				return
			}
			require.NoError(t, err)
			for _, want := range tt.expectContains {
				assert.Contains(t, result, want)
			}
			if cp, ok := tt.provider.(*mockCleanerProvider); ok {
				assert.Equal(t, tt.expectCleaned, cp.cleaned)
			}
		})
	}
}
