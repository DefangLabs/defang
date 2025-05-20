package tools

import (
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
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
