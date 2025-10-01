package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// DefaultEstimateCLI implements EstimateCLIInterface using actual CLI functions
type DefaultEstimateCLI struct{}

func (c *DefaultEstimateCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultEstimateCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (c *DefaultEstimateCLI) RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
	return cli.RunEstimate(ctx, project, client, provider, providerId, region, mode)
}

func (c *DefaultEstimateCLI) PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) {
	cli.PrintEstimate(mode, estimate)
}

func (c *DefaultEstimateCLI) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

func (c *DefaultEstimateCLI) GetRegion(providerId cliClient.ProviderID) string {
	return cliClient.GetRegion(providerId)
}

func (c *DefaultEstimateCLI) CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider {
	return &cliClient.PlaygroundProvider{FabricClient: client}
}

func (c *DefaultEstimateCLI) SetProviderID(providerId *cliClient.ProviderID, providerString string) error {
	return providerId.Set(providerString)
}

func (c *DefaultEstimateCLI) CaptureTermOutput(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) string {
	oldTerm := term.DefaultTerm
	stdout := new(bytes.Buffer)
	term.DefaultTerm = term.NewTerm(
		os.Stdin,
		stdout,
		new(bytes.Buffer),
	)

	cli.PrintEstimate(mode, estimate)

	term.DefaultTerm = oldTerm
	return stdout.String()
}

func handleEstimateTool(ctx context.Context, request mcp.CallToolRequest, providerId *cliClient.ProviderID, cluster string, cli EstimateCLIInterface) (*mcp.CallToolResult, error) {
	term.Debug("Estimate tool called")

	wd, err := request.RequireString("working_directory")
	if err != nil || wd == "" {
		term.Error("Invalid working directory", "error", errors.New("working_directory is required"))
		return mcp.NewToolResultErrorFromErr("Invalid working directory", errors.New("working_directory is required")), err
	}

	err = os.Chdir(wd)
	if err != nil {
		term.Error("Failed to change working directory", "error", err)
		return mcp.NewToolResultErrorFromErr("Failed to change working directory", err), err
	}

	modeString, err := request.RequireString("deployment_mode")
	if err != nil {
		modeString = "AFFORDABLE" // Default to AFFORDABLE if not provided
	}

	providerString, err := request.RequireString("provider")
	if err != nil {
		providerString = providerId.String()
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
		term.Warn("Unknown deployment mode provided, defaulting to AFFORDABLE")
		return mcp.NewToolResultError("Unknown deployment mode provided, please use one of AFFORDABLE, BALANCED or HIGH_AVAILABILITY"), fmt.Errorf("unknown deployment mode: %s", modeString)
	}

	term.Debugf("Deployment mode set to: %s", mode.String())

	loader := cli.ConfigureLoader(request)

	term.Debug("Function invoked: loader.LoadProject")
	project, err := cli.LoadProject(ctx, loader)
	if err != nil {
		err = fmt.Errorf("failed to parse compose file: %w", err)
		term.Error("failed to parse compose file", "error", err)

		return mcp.NewToolResultErrorFromErr("failed to parse compose file", err), err
	}

	term.Debug("Function invoked: cli.Connect")
	client, err := cli.Connect(ctx, cluster)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Could not connect", err), err
	}

	defangProvider := cli.CreatePlaygroundProvider(client)

	var providerID cliClient.ProviderID
	err = cli.SetProviderID(&providerID, providerString)
	if err != nil {
		term.Error("Invalid provider specified", "error", err)
		return mcp.NewToolResultErrorFromErr("Invalid provider specified", err), err
	}

	term.Debug("Function invoked: cli.RunEstimate")
	var region string
	if region == "" {
		region = cli.GetRegion(providerID) // This sets the default region based on the provider
	}

	estimate, err := cli.RunEstimate(ctx, project, client, defangProvider, providerID, region, mode)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("Failed to run estimate", err), err
	}
	term.Debugf("Estimate: %+v", estimate)

	estimateText := cli.CaptureTermOutput(mode, estimate)

	return mcp.NewToolResultText("Successfully estimated the cost of the project to AWS:\n" + estimateText), nil
}
