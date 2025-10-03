package common

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
)

// --- Structs and Types ---
type mockGrpcClientProvider struct {
	client.MockProvider
	setCanIUseConfigCalled bool
	setCanIUseConfigArg    *defangv1.CanIUseResponse
	err                    error
}

type mockGrpcClient struct {
	*cliClient.GrpcClient
	err error
}

// --- Helper Functions ---
func (m *mockGrpcClientProvider) SetCanIUseConfig(resp *defangv1.CanIUseResponse) {
	m.setCanIUseConfigCalled = true
	m.setCanIUseConfigArg = resp
}

func (m *mockGrpcClientProvider) AccountInfo(context.Context) (*cliClient.AccountInfo, error) {
	return &cliClient.AccountInfo{}, m.err
}

func (m *mockGrpcClient) CanIUse(ctx context.Context, req *defangv1.CanIUseRequest) (*defangv1.CanIUseResponse, error) {
	return &defangv1.CanIUseResponse{}, m.err
}

// --- Tests ---
func TestConfigureLoaderBranches(t *testing.T) {
	makeReq := func(args map[string]any) mcp.CallToolRequest {
		return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
	}
	loader1 := ConfigureLoader(makeReq(map[string]any{"project_name": "myproj"}))
	assert.NotNil(t, loader1)
	loader2 := ConfigureLoader(makeReq(map[string]any{"compose_file_paths": []string{"a.yml", "b.yml"}}))
	assert.NotNil(t, loader2)
	loader3 := ConfigureLoader(makeReq(map[string]any{}))
	assert.NotNil(t, loader3)
}

func TestHandleTermsOfServiceError(t *testing.T) {
	origErr := connect.NewError(connect.CodeFailedPrecondition, errors.New("terms of service not accepted"))
	res := HandleTermsOfServiceError(origErr)
	assert.NotNil(t, res)
	otherErr := errors.New("some other error")
	res2 := HandleTermsOfServiceError(otherErr)
	assert.Nil(t, res2)
}

func TestHandleConfigError(t *testing.T) {
	cfgErr := errors.New("missing configs: DB_PASSWORD")
	res := HandleConfigError(cfgErr)
	assert.NotNil(t, res)
	otherErr := errors.New("another error")
	res2 := HandleConfigError(otherErr)
	assert.Nil(t, res2)
}

func TestProviderNotConfiguredError(t *testing.T) {
	err := providerNotConfiguredError(client.ProviderAuto)
	assert.Error(t, err)
	var pid client.ProviderID
	_ = pid.Set("aws")
	err2 := providerNotConfiguredError(pid)
	assert.NoError(t, err2)
}

// --- Test for CanIUseProvider ---

func TestCanIUseProvider(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		grpc := &mockGrpcClient{GrpcClient: &cliClient.GrpcClient{}}
		prov := &mockGrpcClientProvider{}
		err := CanIUseProvider(t.Context(), grpc, client.ProviderAWS, "proj", prov, 2)
		assert.NoError(t, err)
		// No way to check grpc.canIUseCalled since mockGrpcClient does not track it
		// But we can check provider
		assert.True(t, prov.setCanIUseConfigCalled)
	})

	t.Run("grpc error", func(t *testing.T) {
		grpc := &mockGrpcClient{GrpcClient: &cliClient.GrpcClient{}, err: errors.New("fail grpc")}
		prov := &mockGrpcClientProvider{}
		err := CanIUseProvider(t.Context(), grpc, client.ProviderAWS, "proj", prov, 2)
		assert.Error(t, err)
		// No way to check grpc.canIUseCalled since mockGrpcClient does not track it
		assert.False(t, prov.setCanIUseConfigCalled)
	})
}

// Helper to temporarily override newProvider in tests
func withMockedNewProvider(t *testing.T, providerErr error, testFunc func()) {
	originalNewProvider := newProvider
	newProvider = func(_ context.Context, _ cliClient.ProviderID, _ cliClient.FabricClient) (cliClient.Provider, error) {
		return &mockGrpcClientProvider{err: providerErr}, nil
	}
	t.Cleanup(func() { newProvider = originalNewProvider })
	testFunc()
}

func TestCheckProviderConfigured(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name        string
		providerErr error
		grpcErr     error
		wantErr     bool
	}
	testCases := []testCase{
		{
			name:        "success",
			providerErr: nil,
			grpcErr:     nil,
			wantErr:     false,
		},
		{
			name:        "AccountInfo err",
			providerErr: errors.New("account info error"),
			grpcErr:     nil,
			wantErr:     true,
		},
		{
			name:        "CanIUse error",
			providerErr: nil,
			grpcErr:     errors.New("CanIUse error"),
			wantErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withMockedNewProvider(t, tc.providerErr, func() {
				grpc := &mockGrpcClient{GrpcClient: &cliClient.GrpcClient{}, err: tc.grpcErr}
				pid := client.ProviderAWS
				prov, err := CheckProviderConfigured(t.Context(), grpc, pid, "proj", 2)
				if tc.wantErr {
					assert.Error(t, err)
					assert.Nil(t, prov)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, prov)
				}
			})
		})
	}
}
