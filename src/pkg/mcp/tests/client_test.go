package mcp_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
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

type mockFabricService struct {
	defangv1connect.UnimplementedFabricControllerHandler
	configValues map[string]string
}

func (m *mockFabricService) CanIUse(ctx context.Context, req *connect.Request[defangv1.CanIUseRequest]) (*connect.Response[defangv1.CanIUseResponse], error) {
	return connect.NewResponse(&defangv1.CanIUseResponse{CdImage: "beta", Gpu: true}), nil
}

func (m *mockFabricService) WhoAmI(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[defangv1.WhoAmIResponse], error) {
	return connect.NewResponse(&defangv1.WhoAmIResponse{
		Tenant:            "default",
		ProviderAccountId: "default",
		Region:            "us-west-2",
		Tier:              defangv1.SubscriptionTier_HOBBY,
	}), nil
}

func (m *mockFabricService) DeleteSecrets(ctx context.Context, req *connect.Request[defangv1.Secrets]) (*connect.Response[emptypb.Empty], error) {
	for _, name := range req.Msg.Names {
		delete(m.configValues, name)
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) PutSecret(ctx context.Context, req *connect.Request[defangv1.PutConfigRequest]) (*connect.Response[emptypb.Empty], error) {
	m.configValues[req.Msg.Name] = req.Msg.Value
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (m *mockFabricService) ListSecrets(context.Context, *connect.Request[defangv1.ListConfigsRequest]) (*connect.Response[defangv1.Secrets], error) {
	var resp = defangv1.Secrets{}
	for name := range m.configValues {
		resp.Names = append(resp.Names, name)
	}
	return connect.NewResponse(&resp), nil
}

func (m *mockFabricService) GetDelegateSubdomainZone(ctx context.Context, req *connect.Request[defangv1.GetDelegateSubdomainZoneRequest]) (*connect.Response[defangv1.DelegateSubdomainZoneResponse], error) {
	return connect.NewResponse(&defangv1.DelegateSubdomainZoneResponse{
		Zone: "mock-zone.example.com",
	}), nil
}

func (m *mockFabricService) GetServices(ctx context.Context, req *connect.Request[defangv1.GetServicesRequest]) (*connect.Response[defangv1.GetServicesResponse], error) {
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
	// Send a single empty TailResponse for testing purposes
	return stream.Send(&defangv1.TailResponse{})
}

func (m *mockFabricService) Preview(ctx context.Context, req *connect.Request[defangv1.PreviewRequest]) (*connect.Response[defangv1.PreviewResponse], error) {
	return connect.NewResponse(&defangv1.PreviewResponse{}), nil
}

var deployCalled bool

func (m *mockFabricService) Deploy(ctx context.Context, req *connect.Request[defangv1.DeployRequest]) (*connect.Response[defangv1.DeployResponse], error) {
	deployCalled = true
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
	return connect.NewResponse(&emptypb.Empty{}), nil
}

var destroyCalled bool

func (m *mockFabricService) Destroy(ctx context.Context, req *connect.Request[defangv1.DestroyRequest]) (*connect.Response[defangv1.DestroyResponse], error) {
	destroyCalled = true
	return connect.NewResponse(&defangv1.DestroyResponse{
		Etag: "mock-destroy-etag",
	}), nil
}

// --- End mockFabricService methods ---

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

func startMockFabricServer() *httptest.Server {
	mockService := &mockFabricService{
		configValues: make(map[string]string),
	}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	return httptest.NewServer(handler)
}

// startInProcessMCPServer sets up an in-process MCP server and returns a connected client and a stop function.
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

	// initialize the client
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

func beforeTestSuite(ctx context.Context) error {
	var err error
	MCPClientInstance, err = startInProcessMCPServer(ctx, startMockFabricServer())
	if err != nil {
		return err
	}
	return nil
}

func afterTestSuite() {
	if MCPClientInstance != nil {
		MCPClientInstance.cancel()
		MCPClientInstance.fabricServer.Close()
	}
}

func TestInProcessMCPServer_Config(t *testing.T) {
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	defer afterTestSuite()

	client := MCPClientInstance.client

	// set a config value
	var configName = "TEST_VAR"
	_, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "set_config",
			Arguments: map[string]interface{}{
				"working_directory": ".",
				"name":              configName,
				"value":             "test_value",
			},
		},
	})
	if err != nil {
		t.Fatalf("Set Config tool failed: %v", err)
	}

	// list all config values
	result, _ := client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "list_configs",
			Arguments: map[string]interface{}{
				"working_directory": ".",
			},
		},
	})

	// Convert result.Content ([]m3mcp.Content) to a string for searching
	if len(result.Content) == 0 {
		t.Fatalf("List Configs tool returned empty content")
	}

	// verify the content contains the config name we set
	textContent, ok := result.Content[0].(m3mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent type in result.Content[0], got %T", result.Content[0])
	}
	contentStr := textContent.Text
	if !strings.Contains(contentStr, configName) {
		t.Fatalf("Expected config name %q not found in list_configs output: %s", configName, contentStr)
	}

	// remove the config value
	_, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "remove_config",
			Arguments: map[string]interface{}{
				"working_directory": ".",
				"name":              configName,
			},
		},
	})
	if err != nil {
		t.Fatalf("Remove Config tool failed: %v", err)
	}

	// list all config values
	result, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "list_configs",
			Arguments: map[string]interface{}{
				"working_directory": ".",
			},
		},
	})
	if err != nil {
		t.Fatalf("List Configs tool failed: %v", err)
	}

	// verify the content does not contain the config name we set
	textContent, ok = result.Content[0].(m3mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent type in result.Content[0], got %T", result.Content[0])
	}
	contentStr = textContent.Text
	if strings.Contains(contentStr, configName) {
		t.Fatalf("Not expected config name %q not found in list_configs output: %s", configName, contentStr)
	}
}

// --- Begin test functions ---
func TestInProcessMCPServer_Setup(t *testing.T) {
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	defer afterTestSuite()

	client := MCPClientInstance.client

	// validate List resources
	listResourcesReq := m3mcp.ListResourcesRequest{}
	resList, err := client.ListResources(ctx, listResourcesReq)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	t.Logf("Resources: %+v", resList)

	if len(resList.Resources) != 2 {
		t.Fatalf("expected two resources initially, got %d", len(resList.Resources))
	}

	if resList.Resources[0].Name != "defang_dockerfile_and_compose_examples" || resList.Resources[1].Name != "knowledge_base" {
		t.Fatalf("unexpected resource names: %+v", resList.Resources)
	}

	// validate tools list
	listToolsReq := m3mcp.ListToolsRequest{}
	toolListResp, err := client.ListTools(ctx, listToolsReq)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	t.Logf("Tools: %+v", toolListResp)

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

func TestInProcessMCPServer_Services(t *testing.T) {
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	defer afterTestSuite()

	client := MCPClientInstance.client
	tempDir, cleanup := createTestProjectDir(t)
	defer cleanup()

	result, err := client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "services",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	if err != nil {
		t.Fatalf("Services tool failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("Services tool returned an error result: %v", result)
	}

	// Check that the output contains the mock service name
	found := false
	for _, content := range result.Content {
		if text, ok := content.(m3mcp.TextContent); ok {
			if strings.Contains(text.Text, "hello") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("Expected service name 'hello' not found in services tool output: %+v", result.Content)
	}
}

func TestInProcessMCPServer_Estimate(t *testing.T) {
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}
	defer afterTestSuite()

	client := MCPClientInstance.client
	tempDir, cleanup := createTestProjectDir(t)
	defer cleanup()

	result, err := client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "estimate",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
				"provider":          "AWS",
				"deployment_mode":   "AFFORDABLE",
			},
		},
	})
	if err != nil {
		t.Fatalf("Estimate tool failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("Estimate tool returned an error result: %v", result)
	}

	// Check that the output contains the mock cost
	found := false
	for _, content := range result.Content {
		if text, ok := content.(m3mcp.TextContent); ok {
			if strings.Contains(text.Text, "42.42") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("Expected cost '42.42' not found in estimate tool output: %+v", result.Content)
	}
}

func TestInProcessMCPServer_DeployAndDestroy(t *testing.T) {
	ctx := context.Background()
	err := beforeTestSuite(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server : %v", err)
	}
	defer afterTestSuite()

	client := MCPClientInstance.client
	tempDir, cleanup := createTestProjectDir(t)
	defer cleanup()

	// Deploy
	deployCalled = false // reset before call
	result, err := client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "deploy",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	if err != nil {
		t.Fatalf("Deploy tool failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("Deploy tool returned an error result: %v", result)
	}

	if !deployCalled {
		t.Fatalf("Deploy tool was not called on the mock fabric service")
	}

	// Destroy
	destroyCalled = false // reset before call
	_, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "destroy",
			Arguments: map[string]interface{}{
				"working_directory": tempDir,
			},
		},
	})
	if err != nil {
		t.Fatalf("Destroy tool failed: %v", err)
	}

	if !destroyCalled {
		t.Fatalf("Destroy tool was not called on the mock fabric service")
	}
}

// createTestProjectDir creates a temp directory with a simple compose.yaml and returns the dir path and a cleanup function.
func createTestProjectDir(t *testing.T) (string, func()) {
	tempDir, err := ioutil.TempDir("", "defang-testproj-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	cleanup := func() { os.RemoveAll(tempDir) }

	composeContent := `
services:
  hello:
    image: busybox
    command: ["echo", "hello world"]
`
	composePath := filepath.Join(tempDir, "compose.yaml")
	if err := ioutil.WriteFile(composePath, []byte(composeContent), 0644); err != nil {
		cleanup()
		t.Fatalf("failed to write compose.yaml: %v", err)
	}
	return tempDir, cleanup
}
