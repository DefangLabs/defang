package prompts

import (
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestAWSBYOPromptHandler_Success_AccessKey(t *testing.T) {
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
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential":        "AKIAEXAMPLEKEY1234",
				"AWS_SECRET_ACCESS_KEY": "secret",
				"AWS_REGION":            "us-west-2",
			},
		},
	}

	// make sure these env do not exist before the test
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("DEFANG_PROVIDER", "")

	res, err := handler(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, client.ProviderAWS, providerId)
	require.Equal(t, "AKIAEXAMPLEKEY1234", os.Getenv("AWS_ACCESS_KEY_ID"))
	require.Equal(t, "secret", os.Getenv("AWS_SECRET_ACCESS_KEY"))
	require.Equal(t, "us-west-2", os.Getenv("AWS_REGION"))
	require.Equal(t, "aws", os.Getenv("DEFANG_PROVIDER"))
}

func TestAWSBYOPromptHandler_Success_Profile(t *testing.T) {
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
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential": "my-profile",
				"AWS_REGION":     "us-east-1",
			},
		},
	}

	// make sure these env do not exist before the test
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("DEFANG_PROVIDER", "")

	res, err := handler(t.Context(), req)
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Equal(t, client.ProviderAWS, providerId)
	require.Equal(t, "my-profile", os.Getenv("AWS_PROFILE"))
	require.Equal(t, "us-east-1", os.Getenv("AWS_REGION"))
	require.Equal(t, "aws", os.Getenv("DEFANG_PROVIDER"))
}

func TestAWSBYOPromptHandler_MissingSecret(t *testing.T) {
	providerId := client.ProviderID("")
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential": "AKIAEXAMPLEKEY1234",
				"AWS_REGION":     "us-west-2",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.ErrorContains(t, err, "AWS_SECRET_ACCESS_KEY is required")
	require.Nil(t, res)
}

func TestAWSBYOPromptHandler_MissingRegion_AccessKey(t *testing.T) {
	providerId := client.ProviderID("")
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential":        "AKIAEXAMPLEKEY1234",
				"AWS_SECRET_ACCESS_KEY": "secret",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.ErrorContains(t, err, "AWS_REGION is required")
	require.Nil(t, res)
}

func TestAWSBYOPromptHandler_ConnectError(t *testing.T) {
	origConnect := Connect
	Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, os.ErrNotExist }
	defer func() { Connect = origConnect }()

	providerId := client.ProviderID("")
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential":        "AKIAEXAMPLEKEY1234",
				"AWS_SECRET_ACCESS_KEY": "secret",
				"AWS_REGION":            "us-west-2",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.Error(t, err)
	require.Nil(t, res)
}

func TestAWSBYOPromptHandler_CheckProviderConfiguredError(t *testing.T) {
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
	handler := AWSBYOCPromptHandler("test-cluster", &providerId)

	req := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Arguments: map[string]string{
				"AWS Credential":        "AKIAEXAMPLEKEY1234",
				"AWS_SECRET_ACCESS_KEY": "secret",
				"AWS_REGION":            "us-west-2",
			},
		},
	}

	res, err := handler(t.Context(), req)
	require.Error(t, err)
	require.Nil(t, res)
}
