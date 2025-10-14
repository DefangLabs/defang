package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

type EstimateParams struct {
	common.LoaderParams
	DeploymentMode defangv1.DeploymentMode `json:"deployment_mode"`
	Provider       cliClient.ProviderID    `json:"provider"`
	Region         string                  `json:"region"`
}

func ParseEstimateParams(request mcp.CallToolRequest, providerId *cliClient.ProviderID) (EstimateParams, error) {
	modeString, err := request.RequireString("deployment_mode")
	if err != nil {
		modeString = "AFFORDABLE" // Default to AFFORDABLE if not provided
	}

	// This logic is replicated from src/cmd/cli/command/mode.go
	// I couldn't figure out how to import it without circular dependencies
	modeString = strings.ToUpper(modeString)
	var mode defangv1.DeploymentMode
	switch modeString {
	case "AFFORDABLE":
		mode = defangv1.DeploymentMode_DEVELOPMENT
	case "BALANCED":
		mode = defangv1.DeploymentMode_STAGING
	case "HIGH_AVAILABILITY":
		mode = defangv1.DeploymentMode_PRODUCTION
	default:
		term.Warnf("Unknown deployment mode provided - %q", modeString)
		return EstimateParams{}, fmt.Errorf("Unknown deployment mode %q, please use one of %s", modeString, strings.Join(modes.AllDeploymentModes(), ", "))
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

	estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, params.Provider, params.Region, params.DeploymentMode)
	if err != nil {
		return "", fmt.Errorf("Failed to run estimate: %w", err)
	}
	term.Debugf("Estimate: %+v", estimate)

	estimateText := cli.CaptureTermOutput(params.DeploymentMode, estimate)

	return "Successfully estimated the cost of the project to " + params.Provider.Name() + ":\n" + estimateText, nil
}
