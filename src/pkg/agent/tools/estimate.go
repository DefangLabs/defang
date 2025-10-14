package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/mark3labs/mcp-go/mcp"
)

type EstimateParams struct {
	common.LoaderParams
	DeploymentMode modes.Mode           `json:"deployment_mode"`
	Provider       cliClient.ProviderID `json:"provider"`
	Region         string               `json:"region"`
}

func ParseEstimateParams(request mcp.CallToolRequest, providerId *cliClient.ProviderID) (EstimateParams, error) {
	modeString, err := request.RequireString("deployment_mode")
	if err != nil {
		modeString = "AFFORDABLE" // Default to AFFORDABLE if not provided
	}

	mode, err := modes.Parse(modeString) // Validate the mode string
	if err != nil {
		term.Warnf("Unknown deployment mode provided - %q", modeString)
		return EstimateParams{}, fmt.Errorf("unknown deployment mode %q, please use one of %s", modeString, strings.Join(modes.AllDeploymentModes(), ", "))
	}

	providerString, err := request.RequireString("provider")
	if err != nil {
		providerString = "auto" // Default to auto if not provided
	}
	err = providerId.Set(providerString)
	if err != nil {
		return EstimateParams{}, fmt.Errorf("invalid provider specified: %w", err)
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

func HandleEstimateTool(ctx context.Context, loader cliClient.ProjectLoader, params EstimateParams, cluster string, cli CLIInterface) (string, error) {
	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)
		return "", fmt.Errorf("failed to parse compose file: %w", err)
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return "", fmt.Errorf("could not connect: %w", err)
	}

	defangProvider := cli.CreatePlaygroundProvider(client)

	term.Debug("Function invoked: cli.RunEstimate")

	estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, params.Provider, params.Region, params.DeploymentMode)
	if err != nil {
		return "", fmt.Errorf("failed to run estimate: %w", err)
	}
	term.Debugf("Estimate: %+v", estimate)

	estimateText := cli.PrintEstimate(params.DeploymentMode, estimate)

	return "Successfully estimated the cost of the project to " + params.Provider.Name() + ":\n" + estimateText, nil
}
