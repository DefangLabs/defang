package mcp_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	m3client "github.com/mark3labs/mcp-go/client"
	m3mcp "github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	"github.com/DefangLabs/defang/src/pkg/mcp/tools"
	typepb "github.com/DefangLabs/defang/src/protos/google/type"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockFabricService implements the FabricControllerHandler for testing
type mockFabricService struct {
	defangv1connect.UnimplementedFabricControllerHandler
	configValues map[string]string

	// Call tracking for each RPC method
	deleteSecretsCalled bool
	putSecretCalled     bool
	listSecretsCalled   bool
	getServicesCalled   bool
	estimateCalled      bool
	previewCalled       bool
	deployCalled        bool
	destroyCalled       bool

	canIUseCalled                    bool
	whoAmICalled                     bool
	tailCalled                       bool
	putDeploymentCalled              bool
	getDelegateSubdomainZoneCalled   bool
	getPlaygroundProjectDomainCalled bool
}

func (m *mockFabricService) resetFlags() {
	m.deleteSecretsCalled = false
	m.putSecretCalled = false
	m.listSecretsCalled = false
	m.getServicesCalled = false
	m.estimateCalled = false
	m.previewCalled = false
	m.deployCalled = false
	m.destroyCalled = false
	m.canIUseCalled = false
	m.whoAmICalled = false
	m.tailCalled = false
	m.putDeploymentCalled = false
	m.getDelegateSubdomainZoneCalled = false
	m.getPlaygroundProjectDomainCalled = false
}

func (m *mockFabricService) CanIUse(ctx context.Context, req *connect.Request[defangv1.CanIUseRequest]) (*connect.Response[defangv1.CanIUseResponse], error) {
	m.canIUseCalled = true
	return connect.NewResponse(&defangv1.CanIUseResponse{CdImage: "beta", Gpu: true}), nil
}

func (m *mockFabricService) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	m.whoAmICalled = true
	return connect.NewResponse(&defangv1.WhoAmIResponse{
		Tenant: "default",
		Tier:   defangv1.SubscriptionTier_HOBBY,
	}), nil
}

func (m *mockFabricService) DeleteSecrets(ctx context.Context, req *connect.Request[defangv1.Secrets]) (*connect.Response[emptypb.Empty], error) {
	m.deleteSecretsCalled = true
	for _, name := range req.Msg.Names {
		delete(m.configValues, name)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) PutSecret(ctx context.Context, req *connect.Request[defangv1.PutConfigRequest]) (*connect.Response[emptypb.Empty], error) {
	m.putSecretCalled = true
	m.configValues[req.Msg.Name] = req.Msg.Value
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) ListSecrets(context.Context, *connect.Request[defangv1.ListConfigsRequest]) (*connect.Response[defangv1.Secrets], error) {
	m.listSecretsCalled = true
	var resp = defangv1.Secrets{}
	for name := range m.configValues {
		resp.Names = append(resp.Names, name)
	}
	return connect.NewResponse(&resp), nil
}

func (m *mockFabricService) GetDelegateSubdomainZone(ctx context.Context, req *connect.Request[defangv1.GetDelegateSubdomainZoneRequest]) (*connect.Response[defangv1.DelegateSubdomainZoneResponse], error) {
	m.getDelegateSubdomainZoneCalled = true
	return connect.NewResponse(&defangv1.DelegateSubdomainZoneResponse{
		Zone: "mock-zone.example.com",
	}), nil
}

func (m *mockFabricService) GetServices(ctx context.Context, req *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
	m.getServicesCalled = true
	return connect.NewResponse(&defangv1.GetServicesResponse{
		Services: []*defangv1.ServiceInfo{
			{
				Status:     "running",
				PublicFqdn: "http://mock-service.example.com",
				Service: &defangv1.Service{
					Name: "hello",
				},
			},
		},
	}), nil
}

func (m *mockFabricService) Estimate(ctx context.Context, req *connect.Request[defangv1.EstimateRequest]) (*connect.Response[defangv1.EstimateResponse], error) {
	m.estimateCalled = true
	return connect.NewResponse(&defangv1.EstimateResponse{
		Provider: defangv1.Provider_AWS,
		Region:   "us-west-2",
		Subtotal: &typepb.Money{
			CurrencyCode: "USD",
			Units:        42,
			Nanos:        420000000,
		},
		LineItems: []*defangv1.EstimateLineItem{
			{
				Description: "hello service",
				Unit:        "month",
				Quantity:    1,
				Cost: &typepb.Money{
					CurrencyCode: "USD",
					Units:        21,
					Nanos:        210000000,
				},
				Service: []string{"hello"},
			},
			{
				Description: "world service",
				Unit:        "month",
				Quantity:    1,
				Cost: &typepb.Money{
					CurrencyCode: "USD",
					Units:        21,
					Nanos:        210000000,
				},
				Service: []string{"world"},
			},
		},
	}), nil
}

func (m *mockFabricService) Tail(ctx context.Context, req *connect.Request[defangv1.TailRequest], stream *connect.ServerStream[defangv1.TailResponse]) error {
	m.tailCalled = true
	// Send a single empty TailResponse for testing
	return stream.Send(&defangv1.TailResponse{})
}

func (m *mockFabricService) Preview(ctx context.Context, req *connect.Request[defangv1.PreviewRequest]) (*connect.Response[defangv1.PreviewResponse], error) {
	m.previewCalled = true
	return connect.NewResponse(&defangv1.PreviewResponse{}), nil
}

func (m *mockFabricService) Deploy(ctx context.Context, req *connect.Request[defangv1.DeployRequest]) (*connect.Response[defangv1.DeployResponse], error) {
	m.deployCalled = true
	return connect.NewResponse(&defangv1.DeployResponse{
		Services: []*defangv1.ServiceInfo{
			{
				Status:     "mock-deployed",
				PublicFqdn: "http://mock-service.example.com",
				Service: &defangv1.Service{
					Name: "hello",
				},
			},
		},
		Etag: "mock-etag",
	}), nil
}

func (m *mockFabricService) PutDeployment(ctx context.Context, req *connect.Request[defangv1.PutDeploymentRequest]) (*connect.Response[emptypb.Empty], error) {
	m.putDeploymentCalled = true
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) Destroy(ctx context.Context, req *connect.Request[defangv1.DestroyRequest]) (*connect.Response[defangv1.DestroyResponse], error) {
	m.destroyCalled = true
	return connect.NewResponse(&defangv1.DestroyResponse{
		Etag: "mock-destroy-etag",
	}), nil
}

func (m *mockFabricService) GetPlaygroundProjectDomain(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.GetPlaygroundProjectDomainResponse], error) {
	m.getPlaygroundProjectDomainCalled = true
	return connect.NewResponse(&defangv1.GetPlaygroundProjectDomainResponse{
		Domain: "mock-playground.example.com",
	}), nil
}

// End mockFabricService methods

// Test helpers
func setupTest(t *testing.T, tmpDir string) *m3client.Client {
	t.Helper()
	// Create a minimal mock knowledge_base resource in the test's working directory
	resourceDir := tmpDir + "/knowledge_base"
	_ = os.WriteFile(resourceDir+"/README.md", []byte("# Mock Knowledge Base\nThis is a test stub."), 0644)
	mockFabric = &mockFabricService{
		configValues: make(map[string]string),
	}
	fabricServer := startMockFabricServer(mockFabric)
	t.Cleanup(fabricServer.Close)
	mcpClient, err := startInProcessMCPServer(t.Context(), fabricServer)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	t.Cleanup(mcpClient.cancel)
	return mcpClient.client
}

func assertCalled(t *testing.T, called bool, name string) {
	t.Helper()
	require.True(t, called, "%s was not called on the mock fabric service", name)
}

// createTestProjectDir returns a temp directory with a simple compose.yaml and a cleanup function.
func createTestProjectDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	composeContent := `services:
  hello:
    image: busybox
    command: ["echo", "hello world"]
`
	composePath := tempDir + "/compose.yaml"
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		t.Fatalf("failed to write compose.yaml: %v", err)
	}
	return tempDir
}

type testClient struct {
	fabricServer *httptest.Server
	client       *m3client.Client
	serverInfo   *m3mcp.InitializeResult
	cancel       context.CancelFunc
}

// test suite variables
var (
	projectDir string
	mcpClient  *m3client.Client
)

var expectedToolsList = []string{
	"login",
	"services",
	"deploy",
	"destroy",
	"logs",
	"estimate",
	"set_config",
	"remove_config",
	"list_configs",
	"set_aws_provider",
	"set_gcp_provider",
	"set_playground_provider",
}

func startMockFabricServer(mockService *mockFabricService) *httptest.Server {
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	return httptest.NewServer(handler)
}

type cliWithoutBrowser struct {
	tools.DefaultToolCLI
}

func (cliWithoutBrowser) OpenBrowser(url string) error {
	// no-op to avoid opening a browser during tests
	return nil
}

// startInProcessMCPServer sets up an in-process MCP server and returns a connected client and a cleanup function.
func startInProcessMCPServer(ctx context.Context, fabric *httptest.Server) (*testClient, error) {
	providerId := cliClient.ProviderDefang
	cluster := strings.TrimPrefix(fabric.URL, "http://")
	srv, err := mcp.NewDefangMCPServer("0.0.1-test", cluster, &providerId, "", cliWithoutBrowser{})
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %v", err)
	}

	client, err := m3client.NewInProcessClient(srv)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %v", err)
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)

	if err := client.Start(ctxWithTimeout); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start MCP client: %v", err)
	}

	// Initialize the client
	initRequest := m3mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = m3mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = m3mcp.Implementation{
		Name:    "Defang MCP Client",
		Version: "0.0.1-test",
	}
	initRequest.Params.Capabilities = m3mcp.ClientCapabilities{}

	serverInfo, err := client.Initialize(ctxWithTimeout, initRequest)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize MCP client: %v", err)
	}

	return &testClient{
		client:       client,
		fabricServer: fabric,
		serverInfo:   serverInfo,
		cancel:       cancel,
	}, nil
}

var mockFabric *mockFabricService

func TestInProcessMCPServer(t *testing.T) {
	t.Skip()
	TestInProcessMCPServer_Setup := func(t *testing.T) {
		listResourcesReq := m3mcp.ListResourcesRequest{}
		resList, _ := mcpClient.ListResources(t.Context(), listResourcesReq)
		if len(resList.Resources) != 2 {
			t.Fatalf("expected two resources initially, got %d", len(resList.Resources))
		}
		if resList.Resources[0].Name != "defang_dockerfile_and_compose_examples" || resList.Resources[1].Name != "knowledge_base" {
			t.Fatalf("unexpected resource names: %+v", resList.Resources)
		}

		listToolsReq := m3mcp.ListToolsRequest{}
		toolListResp, _ := mcpClient.ListTools(t.Context(), listToolsReq)
		if len(toolListResp.Tools) != len(expectedToolsList) {
			t.Fatalf("expected number of tools: got %d, want %d", len(toolListResp.Tools), len(expectedToolsList))
		}
		missingTools := []string{}
		for _, expected := range expectedToolsList {
			found := false
			for _, tool := range toolListResp.Tools {
				if tool.Name == expected {
					found = true
					break
				}
			}
			if !found {
				missingTools = append(missingTools, expected)
			}
		}
		if len(missingTools) > 0 {
			t.Fatalf("missing expected tools: %v", missingTools)
		}
	}

	// Test functions
	// TestInProcessMCPServer_Services := func(t *testing.T) {
	// 	result, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
	// 		Params: m3mcp.CallToolParams{
	// 			Name: "services",
	// 			Arguments: map[string]interface{}{
	// 				"working_directory": projectDir,
	// 			},
	// 		},
	// 	})
	// 	require.NoError(t, err, "Services tool error")
	// 	assert.True(t, !result.IsError, "Services tool IsError")
	// 	assertCalled(t, mockFabric.getServicesCalled, "services (GetServices)")
	// 	found := false
	// 	for _, content := range result.Content {
	// 		if text, ok := content.(m3mcp.TextContent); ok {
	// 			if strings.Contains(text.Text, "hello") {
	// 				found = true
	// 				break
	// 			}
	// 		}
	// 	}
	// 	assertCalled(t, found, "Expected service name 'hello' in services tool output")
	// }

	TestInProcessMCPServer_Estimate := func(t *testing.T) {
		mockFabric.estimateCalled = false
		start := time.Now()
		result, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "estimate",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
					"provider":          "AWS",
					"deployment_mode":   "AFFORDABLE",
				},
			},
		})
		t.Logf("Estimate tool call took: %v", time.Since(start))
		t.Logf("Estimate tool error: %v", err)
		t.Logf("MockFabric.estimateCalled: %v", mockFabric.estimateCalled)
		require.NoError(t, err, "Estimate tool error")
		assert.True(t, !result.IsError, "Estimate tool IsError")
		assertCalled(t, mockFabric.estimateCalled, "estimate (Estimate)")
		found := false
		for _, content := range result.Content {
			if text, ok := content.(m3mcp.TextContent); ok {
				if strings.Contains(text.Text, "42.42") {
					found = true
					break
				}
			}
		}
		assertCalled(t, found, "Expected cost '42.42' in estimate tool output")
	}

	TestInProcessMCPServer_Login := func(t *testing.T) {
		const dummyToken = "Testing.Token.1234"
		// set as if logged in
		t.Setenv("DEFANG_ACCESS_TOKEN", dummyToken)

		// Call the login tool
		result, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "login",
				Arguments: map[string]interface{}{
					"working_directory": ".",
				},
			},
		})
		require.NoError(t, err, "Login tool error")
		assert.True(t, !result.IsError, "Login tool IsError")
	}

	TestInProcessMCPServer_Config := func(t *testing.T) {
		var configName = "TEST_VAR"
		_, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "set_config",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
					"name":              configName,
					"value":             "test_value",
				},
			},
		})
		if err != nil {
			t.Fatalf("set_config tool failed: %v", err)
		}
		assertCalled(t, mockFabric.putSecretCalled, "set_config (PutSecret)")

		mockFabric.listSecretsCalled = false
		result, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "list_configs",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
				},
			},
		})
		if err != nil {
			t.Fatalf("list_configs tool failed: %v", err)
		}
		assertCalled(t, mockFabric.listSecretsCalled, "list_configs (ListSecrets)")
		if len(result.Content) == 0 {
			t.Fatalf("List Configs tool returned empty content")
		}
		textContent, ok := result.Content[0].(m3mcp.TextContent)
		if !ok {
			t.Fatalf("Expected TextContent type in result.Content[0], got %T", result.Content[0])
		}
		contentStr := textContent.Text
		if !strings.Contains(contentStr, configName) {
			t.Fatalf("Expected config name %q not found in list_configs output: %s", configName, contentStr)
		}

		_, err = mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "remove_config",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
					"name":              configName,
				},
			},
		})
		if err != nil {
			t.Fatalf("remove_config tool failed: %v", err)
		}
		assertCalled(t, mockFabric.deleteSecretsCalled, "remove_config (DeleteSecrets)")

		result, err = mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "list_configs",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
				},
			},
		})
		if err != nil {
			t.Fatalf("list_configs tool failed: %v", err)
		}
		assertCalled(t, mockFabric.listSecretsCalled, "list_configs (ListSecrets after delete)")
		textContent, ok = result.Content[0].(m3mcp.TextContent)
		if !ok {
			t.Fatalf("Expected TextContent type in result.Content[0], got %T", result.Content[0])
		}
		contentStr = textContent.Text
		if strings.Contains(contentStr, configName) {
			t.Fatalf("Not expected config name %q not found in list_configs output: %s", configName, contentStr)
		}
	}

	TestInProcessMCPServer_DeployAndDestroy := func(t *testing.T) {
		const dummyToken = "Testing.Token.1234"
		t.Setenv("DEFANG_ACCESS_TOKEN", dummyToken)

		result, err := mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "deploy",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
				},
			},
		})
		t.Logf("Deploy tool error: %v", err)
		t.Logf("MockFabric.deployCalled: %v", mockFabric.deployCalled)
		require.NoError(t, err, "Deploy tool error")
		assert.True(t, !result.IsError, "Deploy tool IsError")
		assertCalled(t, mockFabric.deployCalled, "deploy (Deploy)")

		_, err = mcpClient.CallTool(t.Context(), m3mcp.CallToolRequest{
			Params: m3mcp.CallToolParams{
				Name: "destroy",
				Arguments: map[string]interface{}{
					"working_directory": projectDir,
				},
			},
		})
		require.NoError(t, err, "Destroy tool error")
		assertCalled(t, mockFabric.destroyCalled, "destroy (Destroy)")
	}

	// Suite-level setup
	projectDir = createTestProjectDir(t)
	mcpClient = setupTest(t, projectDir)

	// Test functions
	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		// {"TestInProcessMCPServer_SetAWSBYOCProvider", TestInProcessMCPServer_SetAWSBYOCProvider},
		// {"TestInProcessMCPServer_SetGCPBYOCProvider", TestInProcessMCPServer_SetGCPBYOCProvider},
		{"TestInProcessMCPServer_DeployAndDestroy", TestInProcessMCPServer_DeployAndDestroy},
		{"TestInProcessMCPServer_Setup", TestInProcessMCPServer_Setup},
		{"TestInProcessMCPServer_Login", TestInProcessMCPServer_Login},
		{"TestInProcessMCPServer_Config", TestInProcessMCPServer_Config},
		{"TestInProcessMCPServer_Estimate", TestInProcessMCPServer_Estimate},
		// TODO: this test was failing on main, so commenting it out. need to mock provider.
		// {"TestInProcessMCPServer_Services", TestInProcessMCPServer_Services},
	}
	for _, tc := range tests {
		mockFabric.resetFlags()
		t.Run(tc.name, tc.fn)
	}
}
