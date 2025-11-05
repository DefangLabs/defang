package tools

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

type EstimateParams struct {
	common.LoaderParams
	DeploymentMode string               `json:"deployment_mode"`
	Provider       cliClient.ProviderID `json:"provider"`
	Region         string               `json:"region"`
}

func ParseEstimateParams(request mcp.CallToolRequest, providerId *cliClient.ProviderID) (EstimateParams, error) {
	mode, err := request.RequireString("deployment_mode")
	if err != nil {
		mode = "AFFORDABLE" // Default to affordable if not provided
	}
	providerString, err := request.RequireString("provider")
	if err != nil {
		providerString = "auto" // Default to auto if not provided
	}
	err = providerId.Set(providerString)
	if err != nil {
		return EstimateParams{}, fmt.Errorf("Invalid provider specified: %w", err)
	}

	var region string
	if region == "" {
		region = cliClient.GetRegion(*providerId) // This sets the default region based on the provider
	}

	return EstimateParams{
		DeploymentMode: mode,
		Provider:       *providerId,
		Region:         region,
	}, nil
}

func HandleEstimateTool(ctx context.Context, loader cliClient.ProjectLoader, params EstimateParams, cluster string, cli EstimateCLIInterface) (string, error) {
	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)
		return "", fmt.Errorf("failed to parse compose file: %w", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("Could not connect: %w", err)
	}

	defangProvider := cli.CreatePlaygroundProvider(client)

	term.Debug("Function invoked: cli.RunEstimate")

	var deploymentMode modes.Mode
	err = deploymentMode.Set(params.DeploymentMode)
	if err != nil {
		return "", err
	}

	estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, params.Provider, params.Region, deploymentMode.Value())
	if err != nil {
		return "", fmt.Errorf("Failed to run estimate: %w", err)
	}
	term.Debugf("Estimate: %+v", estimate)

	estimateText := cli.CaptureTermOutput(deploymentMode.Value(), estimate)

	return "Successfully estimated the cost of the project to " + params.Provider.Name() + ":\n" + estimateText, nil
}
