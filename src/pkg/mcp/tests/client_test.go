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

// startInProcessMCPServer sets up an in-process MCP server and returns a connected client and a stop function.
func startInProcessMCPServer(ctx context.Context) (*MCPClient, error) {
	providerId := cliClient.ProviderDefang
	srv, err := mcp.NewDefangMCPServer("0.0.1-test", "test-cluster", 0, &providerId)
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

func TestInProcessMCPServer_Smoke(t *testing.T) {
	ctx := context.Background()
	clientInfo, err := startInProcessMCPServer(ctx)
	if err != nil {
		t.Fatalf("failed to start in-process MCP server: %v", err)
	}

	defer clientInfo.cancel()

	client := clientInfo.client

	// List resources
	listResourcesReq := m3mcp.ListResourcesRequest{}
	resList, err := client.ListResources(ctx, listResourcesReq)
	if err != nil {
		t.Fatalf("ListResources failed: %v", err)
	}
	t.Logf("Resources: %+v", resList)

	// tools
	listToolsReq := m3mcp.ListToolsRequest{}
	toolList, err := client.ListTools(ctx, listToolsReq)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	t.Logf("Tools: %+v", toolList)
}
