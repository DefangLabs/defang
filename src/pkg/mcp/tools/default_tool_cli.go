package tools

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/pkg/browser"
)

// DefaultToolCLI implements all tool interfaces as passthroughs to the real CLI logic
// This consolidates all Default<tool>CLI structs into one
// Implements: DeployCLIInterface, DestroyCLIInterface, EstimateCLIInterface, ListConfigCLIInterface, LoginCLIInterface, RemoveConfigCLIInterface, SetConfigCLIInterface, CLIInterface, DeploymentInfoInterface

type DefaultToolCLI struct{}

var OpenBrowserFunc = browser.OpenURL

func (DefaultToolCLI) CanIUseProvider(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, provider cliClient.Provider, serviceCount int) error {
	// If there is a real implementation, call it; otherwise, return nil or a stub error
	return nil
}

func (DefaultToolCLI) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return cli.ConfigSet(ctx, projectName, provider, name, value)
}

func (DefaultToolCLI) RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error) {
	return cli.RunEstimate(ctx, project, client, provider, providerId, region, mode)
}
func (DefaultToolCLI) PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) {
	cli.PrintEstimate(mode, estimate)
}

func (DefaultToolCLI) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	req := &defangv1.ListConfigsRequest{Project: projectName}
	return provider.ListConfig(ctx, req)
}

func (DefaultToolCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (DefaultToolCLI) ComposeUp(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, uploadMode compose.UploadMode, mode modes.Mode) (*defangv1.DeployResponse, *compose.Project, error) {
	return cli.ComposeUp(ctx, project, client, provider, uploadMode, mode)
}

func (DefaultToolCLI) Tail(ctx context.Context, provider cliClient.Provider, project *compose.Project, options cli.TailOptions) error {
	return cli.Tail(ctx, provider, project.Name, options)
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

func (DefaultToolCLI) CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error) {
	return common.CheckProviderConfigured(ctx, client, providerId, projectName, serviceCount)
}

func (DefaultToolCLI) CaptureTermOutput(mode modes.Mode, estimate *defangv1.EstimateResponse) string {
	// Use the same logic as DefaultEstimateCLI
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

func (DefaultToolCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (DefaultToolCLI) CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider {
	return &cliClient.PlaygroundProvider{FabricClient: client}
}

func (DefaultToolCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) cliClient.Provider {
	return cli.NewProvider(ctx, providerId, client)
}

func (DefaultToolCLI) OpenBrowser(url string) error {
	if OpenBrowserFunc != nil {
		return OpenBrowserFunc(url)
	}

	return errors.New("no browser function defined")
}

// --- Adapter types for tool interfaces ---
// The following adapter types embed DefaultToolCLI to implement specific tool interfaces.
type DeployCLIAdapter struct{ DefaultToolCLI }
type DestroyCLIAdapter struct{ DefaultToolCLI }
type SetConfigCLIAdapter struct{ DefaultToolCLI }
type RemoveConfigCLIAdapter struct{ DefaultToolCLI }
type ListConfigCLIAdapter struct{ DefaultToolCLI }
type LoginCLIAdapter struct{ DefaultToolCLI }

// --- DestroyCLIInterface ---
func (DestroyCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

// --- SetConfigCLIInterface ---
func (a *SetConfigCLIAdapter) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return a.DefaultToolCLI.ConfigSet(ctx, projectName, provider, name, value)
}
func (a *SetConfigCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

// --- RemoveConfigCLIInterface ---
func (RemoveConfigCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}
func (RemoveConfigCLIAdapter) ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error {
	return cli.ConfigDelete(ctx, projectName, provider, name)
}

// --- ListConfigCLIInterface ---
func (a *ListConfigCLIAdapter) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	return a.DefaultToolCLI.ListConfig(ctx, provider, projectName)
}

// --- LoginCLIInterface ---
func (LoginCLIAdapter) InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string, mcpClient string) error {
	// Delegate to login.InteractiveLoginMCP from the login package
	return login.InteractiveLoginMCP(ctx, client, cluster, mcpClient)
}

func (LoginCLIAdapter) GenerateAuthURL(authPort int) string {
	// Use the same logic as the old DefaultLoginCLI
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}
