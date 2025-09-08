package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SetupResources configures and adds all resources to the MCP server
func SetupResources(s *server.MCPServer) {
	// Create and add documentation resource
	setupDocumentationResource(s)

	// Create and add samples examples resource
	setupSamplesResource(s)
}

var knowledgeBasePath = filepath.Join(client.StateDir, "knowledge_base.json")
var samplesExamplesPath = filepath.Join(client.StateDir, "samples_examples.json")

// setupDocumentationResource configures and adds the documentation resource to the MCP server
func setupDocumentationResource(s *server.MCPServer) {
	term.Info("Creating documentation resource")
	docResource := mcp.NewResource(
		"doc:///knowledge_base/knowledge_base.json",
		"knowledge_base",
		mcp.WithResourceDescription("Defang documentation for any questions or information you need to know about Defang. If you want to build dockerfiles and compose files, please use the defang_dockerfile_and_compose_examples resource or use this as an aid in addition to the defang_dockerfile_and_compose_examples resource."),
		mcp.WithMIMEType("application/json"),
	)

	s.AddResource(docResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Read the file
		file, err := os.ReadFile(knowledgeBasePath)
		if err != nil {
			term.Error("Failed to read resource file", "error", err, "path", "knowledge_base.json")
			return nil, fmt.Errorf("failed to read resource file knowledge_base.json: %w", err)
		}

		// Return the file content
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				Text:     string(file),
				MIMEType: "application/json",
				URI:      "doc:///knowledge_base/knowledge_base.json",
			},
		}, nil
	})
}

// setupSamplesResource configures and adds the samples examples resource to the MCP server
func setupSamplesResource(s *server.MCPServer) {
	term.Info("Creating samples examples resource")
	samplesResource := mcp.NewResource(
		"doc:///knowledge_base/samples_examples.json",
		"defang_dockerfile_and_compose_examples",
		mcp.WithResourceDescription("Defang sample projects that should be used for reference when trying to create new dockerfiles and compose files for defang deploy."),
		mcp.WithMIMEType("application/json"),
	)

	// Add samples examples resource
	s.AddResource(samplesResource, func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		// Read the file
		file, err := os.ReadFile(samplesExamplesPath)
		if err != nil {
			term.Error("Failed to read resource file", "error", err, "path", "samples_examples.json")
			return nil, fmt.Errorf("failed to read resource file samples_examples.json: %w", err)
		}

		// Return the file content
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				Text:     string(file),
				MIMEType: "application/json",
				URI:      "doc:///knowledge_base/samples_examples.json",
			},
		}, nil
	})
}
