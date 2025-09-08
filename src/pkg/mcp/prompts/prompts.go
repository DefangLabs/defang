package prompts

import (
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/mark3labs/mcp-go/server"
)

// SetupPrompts configures and adds all prompts to the MCP server
func SetupPrompts(s *server.MCPServer, cluster string, providerId *client.ProviderID) {
	//AWS BYOC
	setupAWSBYOPrompt(s, cluster, providerId)

	//GCP BYOC
	setupGCPBYOPrompt(s, cluster, providerId)

	//Playground
	setupPlaygroundPrompt(s, providerId)
}
