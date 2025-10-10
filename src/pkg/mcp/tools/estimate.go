package tools

import (
	"context"
	"fmt"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

func handleEstimateTool(ctx context.Context, loader cliClient.ProjectLoader, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli EstimateCLIInterface) (string, error) {
	modeString, err := request.RequireString("deployment_mode")
	if err != nil {
		modeString = "AFFORDABLE" // Default to AFFORDABLE if not provided
	}

	providerString, err := request.RequireString("provider")
	if err != nil {
		providerString = "auto" // Default to auto if not provided
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
		return "", fmt.Errorf("Unknown deployment mode %q, please use one of %s", modeString, strings.Join(modes.AllDeploymentModes(), ", "))
	}

	term.Debugf("Deployment mode set to: %s", mode.String())

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
	err = providerId.Set(providerString)
	if err != nil {
		return "", fmt.Errorf("Invalid provider specified: %w", err)
	}

	term.Debug("Function invoked: cli.RunEstimate")
	var region string
	if region == "" {
		region = cli.GetRegion(*providerId) // This sets the default region based on the provider
	}

	estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, *providerId, region, mode)
	if err != nil {
		return "", fmt.Errorf("Failed to run estimate: %w", err)
	}
	term.Debugf("Estimate: %+v", estimate)

	estimateText := cli.CaptureTermOutput(mode, estimate)

	return "Successfully estimated the cost of the project to " + providerId.Name() + ":\n" + estimateText, nil
}
