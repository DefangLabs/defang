package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

var MCPDevelopmentClient = "" // set by NewDefangMCPServer

const PostPrompt = "Please deploy my application with Defang now."

var ErrNoProviderSet = errors.New("no cloud provider is configured. Use `/` to open prompts and use the 3 Defang setup prompts, or use tools: set_aws_provider, set_gcp_provider, or set_playground_provider.")

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
			return nil, fmt.Errorf("Failed to change working directory: %w", err)
		}
	}

	projectName := params.ProjectName
	if projectName != "" {
		term.Debugf("Project name provided: %s", projectName)
		term.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithProjectName(projectName)), nil
	}
	composeFilePaths := params.ComposeFilePaths
	if len(composeFilePaths) > 0 {
		term.Debugf("Compose file paths provided: %s", composeFilePaths)
		term.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithPath(composeFilePaths...)), nil
	}

	//TODO: Talk about using both project name and compose file paths
	// if projectNameOK && composeFilePathOK {
	// 	term.Infof("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName)
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...)), nil
	// }

	term.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader(), nil
}

func ConfigureLoader(request mcp.CallToolRequest) (*compose.Loader, error) {
	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		return nil, fmt.Errorf("invalid working directory: %w", err)
	}

	err = os.Chdir(wd)
	if err != nil {
		return nil, fmt.Errorf("failed to change working directory: %w", err)
	}

	projectName, err := request.RequireString("project_name")
	if err == nil {
		term.Debugf("Project name provided: %s", projectName)
		term.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithProjectName(projectName)), nil
	}
	arguments := request.GetArguments()
	composeFilePathsArgs, ok := arguments["compose_file_paths"]
	if ok {
		composeFilePaths, ok := composeFilePathsArgs.([]string)
		if ok {
			term.Debugf("Compose file paths provided: %s", composeFilePaths)
			term.Debug("Function invoked: compose.NewLoader")
			return compose.NewLoader(compose.WithPath(composeFilePaths...)), nil
		}
	}

	//TODO: Talk about using both project name and compose file paths
	// if projectNameOK && composeFilePathOK {
	// 	term.Infof("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName)
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...)), nil
	// }

	term.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader(), nil
}

func FixupConfigError(err error) error {
	if strings.Contains(err.Error(), "missing configs") {
		return fmt.Errorf("The operation failed due to missing configs not being set, use the Defang tool called set_config to set the variable: %w", err)
	}
	return err
}

func ProviderNotConfiguredError(providerId client.ProviderID) error {
	if providerId == client.ProviderAuto {
		return ErrNoProviderSet
	}
	return nil
}

func CheckProviderConfigured(ctx context.Context, client cliClient.FabricClient, providerId cliClient.ProviderID, projectName, stack string, serviceCount int) (cliClient.Provider, error) {
	provider := cli.NewProvider(ctx, providerId, client, stack)

	err := cliClient.CanIUseProvider(ctx, client, provider, projectName, stack, serviceCount)
	if err != nil {
		return nil, fmt.Errorf("failed to use provider: %w", err)
	}

	return provider, nil
}
