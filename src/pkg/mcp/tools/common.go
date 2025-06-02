package tools

import (
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

func configureLoader(request mcp.CallToolRequest) *compose.Loader {
	composeFilePaths, composeFilePathOK := request.Params.Arguments["compose_file_paths"].([]string)

	projectName, projectNameOK := request.Params.Arguments["project_name"].(string)

	//TODO: Talk about using both project name and compose file paths

	// if projectNameOK && composeFilePathOK {
	// 	term.Infof("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName)
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...))
	if projectNameOK {
		term.Infof("Project name provided: %s", projectName)
		term.Info("Function invoked: compose.NewLoader")
		term.Info("Function invoked: compose.WithProjectName")
		return compose.NewLoader(compose.WithProjectName(projectName))
	} else if composeFilePathOK {
		term.Infof("Compose file paths provided: %s", composeFilePaths)
		term.Info("Function invoked: compose.NewLoader")
		term.Info("Function invoked: compose.WithPath")
		return compose.NewLoader(compose.WithPath(composeFilePaths...))
	}

	term.Info("Function invoked: compose.NewLoader")
	return compose.NewLoader()
}

// HandleTermsOfServiceError checks if the error is related to terms of service not being accepted
// and returns an appropriate error message if it is.
// Returns nil if the error is not related to terms of service.
func HandleTermsOfServiceError(err error) *mcp.CallToolResult {
	if connect.CodeOf(err) == connect.CodeFailedPrecondition && strings.Contains(err.Error(), "terms of service") {
		return mcp.NewToolResultErrorFromErr("The operation failed because the terms of service were not accepted. Please accept the terms of service by logging in here: https://portal.defang.io/auth/login. Then try again.", err)
	}
	return nil
}
