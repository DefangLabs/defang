package prompts

import (
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestGcpByocPromptHandler_Success(t *testing.T) {
	origConnect := common.Connect
	origCheck := common.CheckProviderConfigured
	common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, nil }
	common.CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
		return &MockProvider{}, nil
	}
	defer func() {
		common.Connect = origConnect
		common.CheckProviderConfigured = origCheck
	}()

	providerId := client.ProviderID("")
	handler := gcpByocPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"GCP_PROJECT_ID": "test-project",
			},
		},
	}

	// make sure these env do not exist before the test
	t.Setenv("GCP_PROJECT_ID", "")
	t.Setenv("DEFANG_PROVIDER", "")

	res, err := handler(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, client.ProviderGCP, providerId)
	require.Equal(t, "test-project", os.Getenv("GCP_PROJECT_ID"))
	require.Equal(t, "gcp", os.Getenv("DEFANG_PROVIDER"))
}

func TestGcpByocPromptHandler_ConnectError(t *testing.T) {
	origConnect := common.Connect
	common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, os.ErrNotExist }
	defer func() { common.Connect = origConnect }()

	providerId := client.ProviderID("")
	handler := gcpByocPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"GCP_PROJECT_ID": "test-project",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.Error(t, err)
	require.Nil(t, res)
}

func TestGcpByocPromptHandler_CheckProviderConfiguredError(t *testing.T) {
	origConnect := common.Connect
	origCheck := common.CheckProviderConfigured
	common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, nil }
	common.CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
		return nil, os.ErrPermission
	}
	defer func() {
		common.Connect = origConnect
		common.CheckProviderConfigured = origCheck
	}()

	providerId := client.ProviderID("")
	handler := gcpByocPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"GCP_PROJECT_ID": "test-project",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.Error(t, err)
	require.Nil(t, res)
}
