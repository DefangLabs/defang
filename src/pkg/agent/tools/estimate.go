package tools

import (
	"context"
	"fmt"
	"strings"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type EstimateParams struct {
	common.LoaderParams
	DeploymentMode string `json:"deployment_mode,omit_empty" jsonschema:"default=affordable,enum=affordable,enum=balanced,enum=high_availability,description=The deployment mode for which to estimate costs (e.g., AFFORDABLE, BALANCED, HIGH_AVAILABILITY)."`
	Provider       string `json:"provider" jsonschema:"required,enum=aws,enum=gcp description=The cloud provider for which to estimate costs."`
	Region         string `json:"region,omit_empty" jsonschema:"description=The region in which to estimate costs."`
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
