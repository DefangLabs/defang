package tools

import (
	"context"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DefaultLoginCLI provides the default implementation
type DefaultLoginCLI struct{}

func (c *DefaultLoginCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultLoginCLI) InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string) error {
	return login.InteractiveLoginMCP(ctx, client, cluster)
}

func (c *DefaultLoginCLI) GenerateAuthURL(authPort int) string {
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}

// handleLoginTool handles the login tool logic
func handleLoginTool(ctx context.Context, request mcp.CallToolRequest, cluster string, authPort int, cli LoginCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Login tool called")
	term.Debugf("mcp.CallToolRequest %+v", request)
	// Test token
	term.Debug("Function invoked: cli.Connect")
	track.Evt("MCP Login Tool", track.P("cluster", cluster), track.P("development_clients", MCPDevelopmentClients))

	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		if authPort != 0 {
			return mcp.NewToolResultText(cli.GenerateAuthURL(authPort)), nil
		}
		term.Debug("Function invoked: cli.InteractiveLoginPrompt")
		err = cli.InteractiveLoginMCP(ctx, client, cluster)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("Failed to login", err), nil
		}
	}

	output := "Successfully logged in to Defang"

	term.Debug(output)
	return mcp.NewToolResultText(output), nil
}

// setupLoginTool configures and adds the login tool to the MCP server
func setupLoginTool(s *server.MCPServer, cluster string, authPort int) {
	term.Debug("Creating login tool")
	loginTool := mcp.NewTool("login",
		mcp.WithDescription("Login to Defang"),
	)
	term.Debug("Login tool created")

	// Add the login tool handler - make it non-blocking
	term.Debug("Adding login tool handler")
	cli := &DefaultToolCLI{}
	s.AddTool(loginTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleLoginTool(ctx, request, cluster, authPort, &LoginCLIAdapter{DefaultToolCLI: cli})
	})
}
