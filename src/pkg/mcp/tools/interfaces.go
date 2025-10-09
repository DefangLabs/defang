// Package tools: consolidated interfaces for mocking CLI/tool dependencies
package tools

import (
	"context"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// --- Common method sets ---
type Connecter interface {
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
}

type ProviderFactory interface {
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error)
}

type LoaderConfigurator interface {
	ConfigureLoader(request mcp.CallToolRequest) cliClient.Loader
}

type ProjectNameLoader interface {
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
}

// --- Tool interfaces composed from common sets ---
type DeployCLIInterface interface {
	Connecter
	ProviderFactory
	// Unique methods
	ComposeUp(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, uploadMode compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *compose.Project, error)
	CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error)
	LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error)
	OpenBrowser(url string) error
}

type LogsCLIInterface interface {
	Connecter
	ProviderFactory
	// Unique methods
	Tail(ctx context.Context, provider cliClient.Provider, project *compose.Project, options cliTypes.TailOptions) error
	CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, serviceCount int) (cliClient.Provider, error)
	LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error)
}

type DestroyCLIInterface interface {
	Connecter
	ProviderFactory
	ProjectNameLoader
	// Unique methods
	ComposeDown(ctx context.Context, projectName string, client *cliClient.GrpcClient, provider cliClient.Provider) (string, error)
	CanIUseProvider(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, provider cliClient.Provider, serviceCount int) error
}

type EstimateCLIInterface interface {
	Connecter
	// Unique methods
	LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error)
	RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode defangv1.DeploymentMode) (*defangv1.EstimateResponse, error)
	PrintEstimate(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse)
	GetRegion(providerId cliClient.ProviderID) string
	CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider
	SetProviderID(providerId *cliClient.ProviderID, providerString string) error
	CaptureTermOutput(mode defangv1.DeploymentMode, estimate *defangv1.EstimateResponse) string
}

type ListConfigCLIInterface interface {
	Connecter
	ProviderFactory
	LoaderConfigurator
	ProjectNameLoader
	// Unique methods
	ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error)
}

type LoginCLIInterface interface {
	Connecter
	// Unique methods
	InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string) error
	GenerateAuthURL(authPort int) string
}

type RemoveConfigCLIInterface interface {
	Connecter
	ProviderFactory
	LoaderConfigurator
	ProjectNameLoader
	// Unique methods
	ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error
}

type SetConfigCLIInterface interface {
	Connecter
	ProjectNameLoader
	// Unique methods
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error)
	ConfigSet(ctx context.Context, projectName string, provider cliClient.Provider, name, value string) error
}

type CLIInterface interface {
	Connecter
	// Unique methods
	GetServices(ctx context.Context, projectName string, provider cliClient.Provider) ([]deployment_info.Service, error)
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient) (cliClient.Provider, error)
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
}
