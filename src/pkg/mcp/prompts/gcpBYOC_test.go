package prompts

import (
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestGCPBYOPromptHandler_Success(t *testing.T) {
	origConnect := Connect
	origCheck := CheckProviderConfigured
	Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, nil }
	CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
		return &MockProvider{}, nil
	}
	defer func() {
		Connect = origConnect
		CheckProviderConfigured = origCheck
	}()

	providerId := client.ProviderID("")
	handler := GCPBYOCPromptHandler("test-cluster", &providerId)

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

	res, err := handler(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, client.ProviderGCP, providerId)
	require.Equal(t, "test-project", os.Getenv("GCP_PROJECT_ID"))
	require.Equal(t, "gcp", os.Getenv("DEFANG_PROVIDER"))
}

func TestGCPBYOPromptHandler_ConnectError(t *testing.T) {
	origConnect := Connect
	Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, os.ErrNotExist }
	defer func() { Connect = origConnect }()

	providerId := client.ProviderID("")
	handler := GCPBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"GCP_PROJECT_ID": "test-project",
			},
		},
	}

	res, err := handler(context.Background(), req)
	require.Error(t, err)
	require.Nil(t, res)
}

func TestGCPBYOPromptHandler_CheckProviderConfiguredError(t *testing.T) {
	origConnect := Connect
	origCheck := CheckProviderConfigured
	Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, nil }
	CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
		return nil, os.ErrPermission
	}
	defer func() {
		Connect = origConnect
		CheckProviderConfigured = origCheck
	}()

	providerId := client.ProviderID("")
	handler := GCPBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"GCP_PROJECT_ID": "test-project",
			},
		},
	}

	res, err := handler(context.Background(), req)
	require.Error(t, err)
	require.Nil(t, res)
}
