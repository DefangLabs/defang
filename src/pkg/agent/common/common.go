package common

import (
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
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

func ConfigureAgentLoader(params LoaderParams, stack *stacks.Parameters) (*compose.Loader, error) {
	if params.WorkingDirectory == "" {
		params.WorkingDirectory = "."
	}

	if params.WorkingDirectory != "." {
		err := os.Chdir(params.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to change working directory: %w", err)
		}
	}

	var opts []compose.LoaderOption

	if stack != nil {
		opts = append(opts, compose.WithInterpolationEnv(map[string]string{
			"DEFANG_PROVIDER": stack.Provider.String(),
			"DEFANG_STACK":    stack.Name,
		}), compose.WithDefaultEnvFiles(session.StackEnvFiles(stack)...))
	}

	if params.ProjectName != "" {
		term.Debugf("Project name provided: %s", params.ProjectName)
		opts = append(opts, compose.WithProjectName(params.ProjectName))
	}

	if len(params.ComposeFilePaths) > 0 {
		term.Debugf("Compose file paths provided: %s", params.ComposeFilePaths)
		opts = append(opts, compose.WithPath(params.ComposeFilePaths...))
	}

	term.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader(opts...), nil
}
