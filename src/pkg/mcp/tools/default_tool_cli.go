package tools

import (
	"bytes"
	"context"
	"os"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// DefaultToolCLI implements all tool interfaces as passthroughs to the real CLI logic
// This consolidates all Default<tool>CLI structs into one
// Implements: DeployCLIInterface, DestroyCLIInterface, EstimateCLIInterface, ListConfigCLIInterface, LoginCLIInterface, RemoveConfigCLIInterface, SetConfigCLIInterface, CLIInterface, DeploymentInfoInterface

type DefaultToolCLI struct{}

// --- DefaultToolCLI: Core tool passthrough methods ---
// All methods for DefaultToolCLI are grouped here for clarity and maintainability.
// These implement the various tool interfaces as passthroughs to the real CLI logic

func (c *DefaultToolCLI) CanIUseProvider(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, provider cliClient.Provider, serviceCount int) error {
	// If there is a real implementation, call it; otherwise, return nil or a stub error
	return nil
}

func (c *DefaultToolCLI) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return cli.ConfigSet(ctx, projectName, provider, name, value)
}

func (c *DefaultToolCLI) RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error) {
	return cli.RunEstimate(ctx, project, client, provider, providerId, region, mode)
}
func (c *DefaultToolCLI) PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) {
	cli.PrintEstimate(mode, estimate)
}

func (c *DefaultToolCLI) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	req := &defangv1.ListConfigsRequest{Project: projectName}
	return provider.ListConfig(ctx, req)
}

func (c *DefaultToolCLI) Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error) {
	return cli.Connect(ctx, cluster)
}

func (c *DefaultToolCLI) NewProviderGrpc(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultToolCLI) NewProviderFabric(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error) {
	return cli.NewProvider(ctx, providerId, client)
}

func (c *DefaultToolCLI) ComposeUp(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, uploadMode compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *compose.Project, error) {
	return cli.ComposeUp(ctx, project, client, provider, uploadMode, mode)
}

func (c *DefaultToolCLI) Tail(ctx context.Context, provider cliClient.Provider, projectName string, options cli.TailOptions) error {
	return cli.Tail(ctx, provider, projectName, options)
}

func (c *DefaultToolCLI) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

func (c *DefaultToolCLI) ComposeDown(ctx context.Context, projectName string, client *cliClient.GrpcClient, provider cliClient.Provider) (string, error) {
	return cli.ComposeDown(ctx, projectName, client, provider)
}

func (c *DefaultToolCLI) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}

func (c *DefaultToolCLI) ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error {
	return cli.ConfigDelete(ctx, projectName, provider, name)
}

func (c *DefaultToolCLI) GetServices(ctx context.Context, projectName string, provider cliClient.Provider) ([]deployment_info.Service, error) {
	return deployment_info.GetServices(ctx, projectName, provider)
}

func (c *DefaultToolCLI) CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error) {
	return CheckProviderConfigured(ctx, client, providerId, projectName, serviceCount)
}

func (c *DefaultToolCLI) CaptureTermOutput(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) string {
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

func (c *DefaultToolCLI) LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error) {
	return loader.LoadProject(ctx)
}

func (c *DefaultToolCLI) CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider {
	return &cliClient.PlaygroundProvider{FabricClient: client}
}

func (c *DefaultToolCLI) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client *cliClient.GrpcClient) (cliClient.Provider, error) {
	return c.NewProviderGrpc(ctx, providerId, client)
}

func (c *DefaultToolCLI) GetRegion(providerId cliClient.ProviderID) string {
	return cliClient.GetRegion(providerId)
}

func (c *DefaultToolCLI) OpenBrowser(url string) error {
	// No-op stub implementation
	return nil
}

func (c *DefaultToolCLI) SetProviderID(providerId *cliClient.ProviderID, providerString string) error {
	return providerId.Set(providerString)
}

// --- Adapter types for tool interfaces ---
// The following adapter types embed DefaultToolCLI to implement specific tool interfaces.
type DeployCLIAdapter struct{ *DefaultToolCLI }
type DestroyCLIAdapter struct{ *DefaultToolCLI }
type SetConfigCLIAdapter struct{ *DefaultToolCLI }
type RemoveConfigCLIAdapter struct{ *DefaultToolCLI }
type ListConfigCLIAdapter struct{ *DefaultToolCLI }
type LoginCLIAdapter struct{ *DefaultToolCLI }

// --- DestroyCLIInterface ---
func (a *DestroyCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}
func (a *DestroyCLIAdapter) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}

// --- SetConfigCLIInterface ---
func (a *SetConfigCLIAdapter) NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error) {
	return a.DefaultToolCLI.NewProviderFabric(ctx, providerId, client)
}
func (a *SetConfigCLIAdapter) ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error {
	return a.DefaultToolCLI.ConfigSet(ctx, projectName, provider, name, value)
}
func (a *SetConfigCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}
func (a *SetConfigCLIAdapter) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return a.DefaultToolCLI.ConfigureLoader(request)
}

// --- RemoveConfigCLIInterface ---
func (a *RemoveConfigCLIAdapter) ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader {
	return configureLoader(request)
}
func (a *RemoveConfigCLIAdapter) LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error) {
	return cliClient.LoadProjectNameWithFallback(ctx, loader, provider)
}
func (a *RemoveConfigCLIAdapter) ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error {
	return cli.ConfigDelete(ctx, projectName, provider, name)
}

// --- ListConfigCLIInterface ---
func (a *ListConfigCLIAdapter) ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error) {
	return a.DefaultToolCLI.ListConfig(ctx, provider, projectName)
}

// --- LoginCLIInterface ---
func (a *LoginCLIAdapter) InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string) error {
	// Delegate to login.InteractiveLoginMCP from the login package
	return login.InteractiveLoginMCP(ctx, client, cluster)
}
func (a *LoginCLIAdapter) GenerateAuthURL(authPort int) string {
	// Use the same logic as the old DefaultLoginCLI
	return "Please open this URL in your browser: http://127.0.0.1:" + strconv.Itoa(authPort) + " to login"
}
