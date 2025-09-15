package mcp_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/mcp"
	m3mcp "github.com/mark3labs/mcp-go/mcp"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/client"
)

type MCPClient struct {
	client     *client.Client
	serverInfo *m3mcp.InitializeResult
	cancel     context.CancelFunc
}

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

// startInProcessMCPServer sets up an in-process MCP server and returns a connected client and a stop function.
func startInProcessMCPServer(ctx context.Context) (*MCPClient, error) {
	providerId := cliClient.ProviderDefang
	//TODO: look at mocking out fabric
	srv, err := mcp.NewDefangMCPServer("0.0.1-test", "fabric-prod1.defang.dev", 0, &providerId)
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
		client:     client,
		serverInfo: serverInfo,
		cancel:     cancel,
	}, nil
}

func TestInProcessMCPServer_Setup(t *testing.T) {
	ctx := context.Background()
	clientInfo, err := startInProcessMCPServer(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}

	defer clientInfo.cancel()

	client := clientInfo.client

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

func TestInProcessMCPServer_Login(t *testing.T) {
	ctx := context.Background()
	clientInfo, err := startInProcessMCPServer(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}

	defer clientInfo.cancel()

	client := clientInfo.client
	_, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "login",
		},
	})
	if err != nil {
		t.Fatalf("Login tool failed!!!: %v", err)
	}

	_, err = client.CallTool(ctx, m3mcp.CallToolRequest{
		Params: m3mcp.CallToolParams{
			Name: "set_config",
			Arguments: map[string]interface{}{
				"working_directory": ".",
				"name":              "TEST_VAR",
				"value":             "test_value",
			},
		},
	})
	if err != nil {
		t.Fatalf("Set Config tool failed: %v", err)
	}

	result, err := client.CallTool(ctx, m3mcp.CallToolRequest{
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

	t.Logf("List Configs Result: %+v", result)
}
