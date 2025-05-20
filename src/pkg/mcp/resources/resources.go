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

	// Create and add sample prompt
	setupSamplePrompt(s)
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

// setupSamplePrompt configures and adds the sample prompt to the MCP server
func setupSamplePrompt(s *server.MCPServer) {
	samplePrompt := mcp.NewPrompt("Make Dockerfile and compose file",
		mcp.WithPromptDescription("The user should give you a path to a project directory, and you should create a Dockerfile and compose file for that project. If there is an app folder, make the Dockerfile for that folder. Then make a compose file for original project directory or root of that project directory."),
		mcp.WithArgument("project_path",
			mcp.ArgumentDescription("Path to the project directory"),
			mcp.RequiredArgument(),
		),
	)

	s.AddPrompt(samplePrompt, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		projectPath, ok := request.Params.Arguments["project_path"]
		if !ok || projectPath == "" {
			projectPath = "."
			term.Warn("Project path not provided, using current directory", "dir", projectPath)
		}

		return mcp.NewGetPromptResult(
			"Code assistance to make Dockerfile and compose file",
			[]mcp.PromptMessage{
				mcp.NewPromptMessage(
					mcp.RoleUser,
					mcp.NewTextContent(fmt.Sprintf("You are a helpful code writer. I will give you a path which is %s to a project directory, and you should create a Dockerfile and compose file for that project. If there is an app folder, make the Dockerfile for that folder. Then make a compose file for original project directory or root of that project directory. When creating these files, make sure to use the samples and examples resource for reference of defang. If you need more information, please use the defang documentation resource. When you are creating these files please make sure to scan carefully to expose any ports, start commands, and any other information needed for the project.", projectPath)),
				),
				mcp.NewPromptMessage(
					mcp.RoleAssistant,
					mcp.NewEmbeddedResource(mcp.TextResourceContents{
						MIMEType: "application/json",
						URI:      "doc:///knowledge_base/knowledge_base.json",
					}),
				),
			},
		), nil
	})
}
