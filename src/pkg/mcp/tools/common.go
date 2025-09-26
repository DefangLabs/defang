package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
	"github.com/mark3labs/mcp-go/mcp"
)

var MCPDevelopmentClient = ""

var newProvider = cli.NewProvider

func configureLoader(request mcp.CallToolRequest) *compose.Loader {
	projectName, err := request.RequireString("project_name")
	if err == nil {
		term.Debugf("Project name provided: %s", projectName)
		term.Debug("Function invoked: compose.NewLoader")
		return compose.NewLoader(compose.WithProjectName(projectName))
	}
	arguments := request.GetArguments()
	composeFilePathsArgs, ok := arguments["compose_file_paths"]
	if ok {
		composeFilePaths, ok := composeFilePathsArgs.([]string)
		if ok {
			term.Debugf("Compose file paths provided: %s", composeFilePaths)
			term.Debug("Function invoked: compose.NewLoader")
			return compose.NewLoader(compose.WithPath(composeFilePaths...))
		}
	}

	//TODO: Talk about using both project name and compose file paths
	// if projectNameOK && composeFilePathOK {
	// 	term.Infof("Compose file paths and project name provided: %s, %s", composeFilePaths, projectName)
	// 	return compose.NewLoader(compose.WithProjectName(projectName), compose.WithPath(composeFilePaths...))
	// }

	term.Debug("Function invoked: compose.NewLoader")
	return compose.NewLoader()
}

// HandleTermsOfServiceError checks if the error is related to terms of service not being accepted
// and returns an appropriate error message if it is.
// Returns nil if the error is not related to terms of service.
func HandleTermsOfServiceError(err error) *mcp.CallToolResult {
	if connect.CodeOf(err) == connect.CodeFailedPrecondition && strings.Contains(err.Error(), "terms of service") {
		mcpResult := mcp.NewToolResultErrorFromErr("The operation failed because the terms of service were not accepted. Please accept the terms of service by logging in here: https://portal.defang.io/auth/login. Then try again.", err)
		term.Debugf("MCP output error: %v", mcpResult)
		return mcpResult
	}
	return nil
}

func HandleConfigError(err error) *mcp.CallToolResult {
	if strings.Contains(err.Error(), "missing configs") {
		mcpResult := mcp.NewToolResultErrorFromErr("The operation failed due to missing configs not being set. Please use the Defang tool called set_config to set the variable.", err)
		term.Debugf("MCP output error: %v", mcpResult)
		return mcpResult
	}
	return nil
}

func CanIUseProvider(ctx context.Context, grpcClient client.FabricClient, providerId client.ProviderID, projectName string, provider client.Provider, serviceCount int) error {
	canUseReq := defangv1.CanIUseRequest{
		Project:      projectName,
		Provider:     providerId.Value(),
		ServiceCount: int32(serviceCount), // #nosec G115 - service count will not overflow int32
	}
	term.Debug("Function invoked: client.CanIUse")
	resp, err := grpcClient.CanIUse(ctx, &canUseReq)
	if err != nil {
		return err
	}

	term.Debug("Function invoked: provider.SetCanIUseConfig")
	provider.SetCanIUseConfig(resp)
	return nil
}

func providerNotConfiguredError(providerId client.ProviderID) error {
	if providerId == client.ProviderAuto {
		term.Error("No provider configured")
		return errors.New("no provider is configured; please type in the chat /defang.AWS_Setup for AWS, /defang.GCP_Setup for GCP, or /defang.Playground_Setup for Playground.")
	}
	return nil
}

func CheckProviderConfigured(ctx context.Context, client cliClient.FabricClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error) {
	provider, err := newProvider(ctx, providerId, client)
	if err != nil {
		term.Error("Failed to get new provider", "error", err)
		return nil, err
	}

	_, err = provider.AccountInfo(ctx)
	if err != nil {
		return nil, err
	}

	err = CanIUseProvider(ctx, client, providerId, projectName, provider, serviceCount)
	if err != nil {
		term.Error("Failed to use provider", "error", err)
		return nil, err
	}

	return provider, nil
}
