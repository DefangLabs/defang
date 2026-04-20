package common

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
)

var MCPDevelopmentClient = "" // set by NewDefangMCPServer

const PostPrompt = "Please deploy my application with Defang now."

var ErrNoProviderSet = errors.New("no cloud provider is configured.")

func GetStringArg(args map[string]string, key, defaultValue string) string {
	if val, exists := args[key]; exists {
		return val
	}
	return defaultValue
}

type LoaderParams struct {
	WorkingDirectory string   `json:"working_directory" jsonschema:"description=The working directory containing the compose files. Usually the current directory."`
	ProjectName      string   `json:"project_name,omitempty" jsonschema:"description=Optional: The name of the project. Useful when working with projects that are not in the current directory."`
	ComposeFilePaths []string `json:"compose_file_paths,omitempty" jsonschema:"description=Optional: Paths to the compose files to use for the project. If not provided, defaults to the compose file in the working directory."`
}

func ConfigureAgentLoader(params LoaderParams) (*compose.Loader, error) {
	if params.WorkingDirectory == "" {
		params.WorkingDirectory = "."
	}

	if params.WorkingDirectory != "." {
		err := os.Chdir(params.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to change working directory: %w", err)
		}
	}

	projectName := params.ProjectName
	if projectName != "" {
		slog.Debug("Project name provided: " + projectName)
		slog.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithProjectName(projectName)), nil
	}
	composeFilePaths := params.ComposeFilePaths
	if len(composeFilePaths) > 0 {
		slog.Debug(fmt.Sprintf("Compose file paths provided: %v", composeFilePaths))
		slog.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithPath(composeFilePaths...)), nil
	}

	//TODO: Talk about using both project name and compose file paths
	// if projectNameOK && composeFilePathOK {
	// 	slog.Info(fmt.Sprintf("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName))
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...)), nil
	// }

	slog.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader(), nil
}
