package tools

import (
	"bytes"
	"context"
	"os"
	"strconv"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type StackConfig struct {
	Cluster    string
	ProviderID *cliClient.ProviderID
	Stack      string
}

// DefaultToolCLI implements all tool interfaces as passthroughs to the real CLI logic
// This consolidates all Default<tool>CLI structs into one
// Implements: CLIInterface

type DefaultToolCLI struct{}

func (DefaultToolCLI) CanIUseProvider(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, provider cliClient.Provider, serviceCount int) error {
	return cliClient.CanIUseProvider(ctx, client, provider, projectName, "", serviceCount) // TODO: add stack
}

func (DefaultToolCLI) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return cli.ConfigSet(ctx, projectName, provider, name, value)
}

func (DefaultToolCLI) RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	return cli.RunEstimate(ctx, project, client, provider, providerId, region, mode)
}

func (DefaultToolCLI) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	req := &defangv1.ListConfigsRequest{Project: projectName}
	return provider.ListConfig(ctx, req)
}

func (DefaultToolCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (DefaultToolCLI) ComposeUp(ctx context.Context, client *cliClient.GrpcClient, provider cliClient.Provider, params cli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	return cli.ComposeUp(ctx, client, provider, params)
}

func (DefaultToolCLI) Tail(ctx context.Context, provider cliClient.Provider, projectName string, options cli.TailOptions) error {
	return cli.Tail(ctx, provider, projectName, options)
}

func (DefaultToolCLI) ComposeDown(ctx context.Context, projectName string, client *cliClient.GrpcClient, provider cliClient.Provider) (string, error) {
	return cli.ComposeDown(ctx, projectName, client, provider)
}

func (DefaultToolCLI) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

func (DefaultToolCLI) ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error {
	return cli.ConfigDelete(ctx, projectName, provider, name)
}

func (DefaultToolCLI) GetServices(ctx context.Context, projectName string, provider cliClient.Provider) ([]deployment_info.Service, error) {
	return deployment_info.GetServices(ctx, projectName, provider)
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

func (DefaultToolCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (DefaultToolCLI) CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider {
	return &cliClient.PlaygroundProvider{FabricClient: client}
}

func (DefaultToolCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient, stack string) cliClient.Provider {
	return cli.NewProvider(ctx, providerId, client, stack)
}

func (DefaultToolCLI) GenerateAuthURL(authPort int) string {
	// Use the same logic as the old DefaultLoginCLI
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}

func (DefaultToolCLI) InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string, mcpClient string) error {
	return login.InteractiveLoginMCP(ctx, client, cluster, mcpClient)
}

func (DefaultToolCLI) TailAndMonitor(ctx context.Context, project *compose.Project, provider cliClient.Provider, waitTimeout time.Duration, options cli.TailOptions) (cli.ServiceStates, error) {
	return cli.TailAndMonitor(ctx, project, provider, waitTimeout, options)
}
