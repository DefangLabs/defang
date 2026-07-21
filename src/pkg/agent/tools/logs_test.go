package tools

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLogsCLI struct {
	CLIInterface
	callLog     []string
	tailProject string
	tailOptions cliTypes.TailOptions
}

func (m *mockLogsCLI) Connect(context.Context, string) (*client.GrpcClient, error) {
	m.callLog = append(m.callLog, "Connect")
	return &client.GrpcClient{}, nil
}

func (m *mockLogsCLI) NewProvider(context.Context, client.ProviderID, client.FabricClient, string) client.Provider {
	m.callLog = append(m.callLog, "NewProvider")
	return client.MockProvider{}
}

func (m *mockLogsCLI) LoadProjectNameWithFallback(context.Context, client.Loader, client.Provider) (string, error) {
	m.callLog = append(m.callLog, "LoadProjectNameWithFallback")
	return "test-project", nil
}

func (m *mockLogsCLI) CanIUseProvider(context.Context, *client.GrpcClient, client.Provider, string, int) error {
	m.callLog = append(m.callLog, "CanIUseProvider")
	return nil
}

func (m *mockLogsCLI) Tail(_ context.Context, _ client.Provider, projectName string, options cliTypes.TailOptions) error {
	m.callLog = append(m.callLog, "Tail")
	m.tailProject = projectName
	m.tailOptions = options
	return nil
}

func TestHandleLogsTool(t *testing.T) {
	t.Chdir(t.TempDir())
	mockCLI := &mockLogsCLI{}
	stack := &stacks.Parameters{Name: "production", Provider: client.ProviderAWS}
	ec := elicitations.NewController(&mockElicitationsClient{})

	result, err := HandleLogsTool(t.Context(), &client.MockLoader{}, LogsParams{
		LoaderParams: common.LoaderParams{WorkingDirectory: "."},
		DeploymentID: "deployment-123",
		Since:        "1h",
		Until:        "30m",
	}, mockCLI, ec, StackConfig{FabricAddr: "test-cluster", Stack: stack})

	require.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, []string{
		"Connect",
		"NewProvider",
		"LoadProjectNameWithFallback",
		"CanIUseProvider",
		"Tail",
	}, mockCLI.callLog)
	assert.Equal(t, "test-project", mockCLI.tailProject)
	assert.Equal(t, "deployment-123", mockCLI.tailOptions.Deployment)
	assert.Equal(t, "production", mockCLI.tailOptions.Stack)
	assert.Equal(t, int32(100), mockCLI.tailOptions.Limit)
	assert.True(t, mockCLI.tailOptions.PrintBookends)
	assert.True(t, mockCLI.tailOptions.Verbose)
	assert.False(t, mockCLI.tailOptions.Since.IsZero())
	assert.False(t, mockCLI.tailOptions.Until.IsZero())
	assert.True(t, mockCLI.tailOptions.Since.Before(mockCLI.tailOptions.Until), "since %v should be before until %v", mockCLI.tailOptions.Since, mockCLI.tailOptions.Until)
}
