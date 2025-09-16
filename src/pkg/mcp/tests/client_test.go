package mcp_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/client"
	m3mcp "github.com/mark3labs/mcp-go/mcp"
	"google.golang.org/protobuf/types/known/emptypb"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp"
	typepb "github.com/DefangLabs/defang/src/protos/google/type"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
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

	canIUseCalled                  bool
	whoAmICalled                   bool
	tailCalled                     bool
	putDeploymentCalled            bool
	getDelegateSubdomainZoneCalled bool
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

// End mockFabricService methods

// Test helpers
func setupTest(t *testing.T) (*client.Client, func()) {
	t.Helper()
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	cleanup := func() { afterTestSuite() }
	return MCPClientInstance.client, cleanup
}

func assertCalled(t *testing.T, called bool, name string) {
	t.Helper()
	if !called {
		t.Fatalf("%s was not called on the mock fabric service", name)
	}
}

// createTestProjectDir returns a temp directory with a simple compose.yaml and a cleanup function.
func createTestProjectDir(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := ioutil.TempDir("", "defang-testproj-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	composeContent := `services:
  hello:
    image: busybox
    command: ["echo", "hello world"]
`
	composePath := tempDir + "/compose.yaml"
	if err := os.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to write compose.yaml: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tempDir) })
	t.Chdir(tempDir)
	return tempDir, func() {}
}

type MCPClient struct {
	fabricServer *httptest.Server
	client       *client.Client
	serverInfo   *m3mcp.InitializeResult
	cancel       context.CancelFunc
}

var MCPClientInstance *MCPClient
var expectedToolsList = []string{
	"login",
	"services",
	"deploy",
	"destroy",
	"estimate",
	"set_config",
	"remove_config",
	"list_configs",
}

func startMockFabricServer(mockService *mockFabricService) *httptest.Server {
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	return httptest.NewServer(handler)
}

// startInProcessMCPServer sets up an in-process MCP server and returns a connected client and a cleanup function.
func startInProcessMCPServer(ctx context.Context, fabric *httptest.Server) (*MCPClient, error) {
	providerId := cliClient.ProviderDefang
	//TODO: look at mocking out fabric
	cluster := strings.TrimPrefix(fabric.URL, "http://")
	srv, err := mcp.NewDefangMCPServer("0.0.1-test", cluster, 0, &providerId)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %v", err)
	}

	client, err := client.NewInProcessClient(srv)
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

	return &MCPClient{
		client:       client,
		fabricServer: fabric,
		serverInfo:   serverInfo,
		cancel:       cancel,
	}, nil
}

var MockFabric *mockFabricService

func beforeTestSuite(ctx context.Context) error {
	MockFabric = &mockFabricService{
		configValues: make(map[string]string),
	}
	var err error
	MCPClientInstance, err = startInProcessMCPServer(ctx, startMockFabricServer(MockFabric))
	if err != nil {
		return err
	}

	MockFabric.resetFlags()
	return nil
}

func afterTestSuite() {
	if MCPClientInstance != nil {
		MCPClientInstance.cancel()
		MCPClientInstance.fabricServer.Close()
	}
}

func TestInProcessMCPServer_Config(t *testing.T) {
	client, cleanup := setupTest(t)
	defer cleanup()

	var configName = "TEST_VAR"
	_, _ = client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "set_config",
			Arguments: map[string]interface{}{
				"working_directory": ".",
				"name":              configName,
				"value":             "test_value",
			},
		},
	})
	assertCalled(t, MockFabric.putSecretCalled, "set_config (PutSecret)")

	MockFabric.listSecretsCalled = false
	result, _ := client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "list_configs",
			Arguments: map[string]interface{}{
				"working_directory": ".",
			},
		},
	})
	assertCalled(t, MockFabric.listSecretsCalled, "list_configs (ListSecrets)")
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

	_, _ = client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "remove_config",
			Arguments: map[string]interface{}{
				"working_directory": ".",
				"name":              configName,
			},
		},
	})
	assertCalled(t, MockFabric.deleteSecretsCalled, "remove_config (DeleteSecrets)")

	result, err := client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "list_configs",
			Arguments: map[string]interface{}{
				"working_directory": ".",
			},
		},
	})
	assertCalled(t, MockFabric.listSecretsCalled, "list_configs (ListSecrets after delete)")
	if err != nil {
		t.Fatalf("List Configs tool failed: %v", err)
	}
	textContent, ok = result.Content[0].(m3mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent type in result.Content[0], got %T", result.Content[0])
	}
	contentStr = textContent.Text
	if strings.Contains(contentStr, configName) {
		t.Fatalf("Not expected config name %q not found in list_configs output: %s", configName, contentStr)
	}
}

// Test functions
func TestInProcessMCPServer_Setup(t *testing.T) {
	client, cleanup := setupTest(t)
	defer cleanup()

	listResourcesReq := m3mcp.ListResourcesRequest{}
	resList, _ := client.ListResources(context.Background(), listResourcesReq)
	if len(resList.Resources) != 2 {
		t.Fatalf("expected two resources initially, got %d", len(resList.Resources))
	}
	if resList.Resources[0].Name != "defang_dockerfile_and_compose_examples" || resList.Resources[1].Name != "knowledge_base" {
		t.Fatalf("unexpected resource names: %+v", resList.Resources)
	}

	listToolsReq := m3mcp.ListToolsRequest{}
	toolListResp, _ := client.ListTools(context.Background(), listToolsReq)
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
func TestInProcessMCPServer_Services(t *testing.T) {
	client, cleanup := setupTest(t)
	tempDir, cleanupDir := createTestProjectDir(t)
	defer cleanup()
	defer cleanupDir()

	result, err := client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "services",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	assertCalled(t, err == nil, "Services tool error")
	assertCalled(t, !result.IsError, "Services tool IsError")
	assertCalled(t, MockFabric.getServicesCalled, "services (GetServices)")
	found := false
	for _, content := range result.Content {
		if text, ok := content.(m3mcp.TextContent); ok {
			if strings.Contains(text.Text, "hello") {
				found = true
				break
			}
		}
	}
	assertCalled(t, found, "Expected service name 'hello' in services tool output")
}

func TestInProcessMCPServer_Estimate(t *testing.T) {
	client, cleanup := setupTest(t)
	tempDir, cleanupDir := createTestProjectDir(t)
	defer cleanup()
	defer cleanupDir()

	result, err := client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "estimate",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
				"provider":          "AWS",
				"deployment_mode":   "AFFORDABLE",
			},
		},
	})
	assertCalled(t, err == nil, "Estimate tool error")
	assertCalled(t, !result.IsError, "Estimate tool IsError")
	assertCalled(t, MockFabric.estimateCalled, "estimate (Estimate)")
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

func TestInProcessMCPServer_DeployAndDestroy(t *testing.T) {
	client, cleanup := setupTest(t)
	tempDir, cleanupDir := createTestProjectDir(t)
	defer cleanup()
	defer cleanupDir()

	result, err := client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "deploy",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	assertCalled(t, err == nil, "Deploy tool error")
	assertCalled(t, !result.IsError, "Deploy tool IsError")
	assertCalled(t, MockFabric.deployCalled, "deploy (Deploy)")

	_, err = client.CallTool(context.Background(), m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "destroy",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	assertCalled(t, err == nil, "Destroy tool error")
	assertCalled(t, MockFabric.destroyCalled, "destroy (Destroy)")
}
