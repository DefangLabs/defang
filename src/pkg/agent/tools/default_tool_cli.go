package tools

import (
	"bytes"
	"context"
	"os"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type StackConfig struct {
	Cluster string
	Stack   *stacks.StackParameters
}

// DefaultToolCLI implements all tool interfaces as passthroughs to the real CLI logic
// This consolidates all Default<tool>CLI structs into one
// Implements: CLIInterface

type DefaultToolCLI struct{}

func (DefaultToolCLI) CanIUseProvider(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, projectName string, serviceCount int) error {
	return client.CanIUseProvider(ctx, fabric, provider, projectName, serviceCount)
}

func (DefaultToolCLI) ConfigSet(ctx context.Context, projectName string, provider client.Provider, name, value string) error {
	return cli.ConfigSet(ctx, projectName, provider, name, value)
}

func (DefaultToolCLI) RunEstimate(ctx context.Context, project *compose.Project, fabric *client.GrpcClient, provider client.Provider, providerId client.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	return cli.RunEstimate(ctx, project, fabric, provider, providerId, region, mode)
}

func (DefaultToolCLI) ListConfig(ctx context.Context, provider client.Provider, projectName string) (*defangv1.Secrets, error) {
	req := &defangv1.ListConfigsRequest{Project: projectName}
	return provider.ListConfig(ctx, req)
}

func (DefaultToolCLI) Connect(ctx context.Context, cluster string) (*client.GrpcClient, error) {
	// TODO: add workspace support to the MCP server
	return cli.ConnectWithTenant(ctx, cluster, types.TenantUnset)
}

func (DefaultToolCLI) ComposeUp(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, stack *stacks.StackParameters, params cli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	return cli.ComposeUp(ctx, fabric, provider, stack, params)
}

func (DefaultToolCLI) Tail(ctx context.Context, provider client.Provider, projectName string, options cli.TailOptions) error {
	return cli.Tail(ctx, provider, projectName, options)
}

func (DefaultToolCLI) ComposeDown(ctx context.Context, projectName string, fabric *client.GrpcClient, provider client.Provider) (string, error) {
	return cli.ComposeDown(ctx, projectName, fabric, provider)
}

func (DefaultToolCLI) LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error) {
	return client.LoadProjectNameWithFallback(ctx, loader, provider)
}

func (DefaultToolCLI) ConfigDelete(ctx context.Context, projectName string, provider client.Provider, name string) error {
	return cli.ConfigDelete(ctx, projectName, provider, name)
}

func (DefaultToolCLI) GetServices(ctx context.Context, projectName string, provider client.Provider) ([]cli.Service, error) {
	servicesResponse, err := cli.GetServices(ctx, projectName, provider)
	if err != nil {
		return nil, err
	}

	si, err := cli.GetServiceStatesAndEndpoints(servicesResponse.Services)
	return si, err
}

func (DefaultToolCLI) PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) string {
	stdout := new(bytes.Buffer)
	captureTerm := term.NewTerm(
		os.Stdin,
		stdout,
		new(bytes.Buffer),
	)

	cli.PrintEstimate(mode, estimate, captureTerm)

	return stdout.String()
}

func (DefaultToolCLI) LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (DefaultToolCLI) CreatePlaygroundProvider(fabric *client.GrpcClient) client.Provider {
	return &client.PlaygroundProvider{FabricClient: fabric}
}

func (DefaultToolCLI) NewProvider(ctx context.Context, providerId client.ProviderID, fabric client.FabricClient, stack string) client.Provider {
	return cli.NewProvider(ctx, providerId, fabric, stack)
}

func (DefaultToolCLI) GenerateAuthURL(authPort int) string {
	// Use the same logic as the old DefaultLoginCLI
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}

func (DefaultToolCLI) InteractiveLoginMCP(ctx context.Context, cluster string, mcpClient string) error {
	return login.InteractiveLoginMCP(ctx, cluster, mcpClient)
}
