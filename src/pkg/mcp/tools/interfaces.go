// Package tools: consolidated interfaces for mocking CLI/tool dependencies
package tools

import (
	"context"

	cliTypes "github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/mcp/deployment_info"
	"github.com/DefangLabs/defang/src/pkg/modes"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type CLIInterface interface {
	CanIUseProvider(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName string, provider cliClient.Provider, serviceCount int) error
	CheckProviderConfigured(ctx context.Context, client *cliClient.GrpcClient, providerId cliClient.ProviderID, projectName, stack string, serviceCount int) (cliClient.Provider, error)
	ComposeDown(ctx context.Context, projectName string, client *cliClient.GrpcClient, provider cliClient.Provider) (string, error)
	ComposeUp(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, uploadMode compose.UploadMode, mode modes.Mode) (*defangv1.DeployResponse, *compose.Project, error)
	ConfigDelete(ctx context.Context, projectName string, provider cliClient.Provider, name string) error
	ConfigSet(ctx context.Context, isSecret bool, projectName string, provider cliClient.Provider, name, value string) error
	Connect(ctx context.Context, cluster string) (*cliClient.GrpcClient, error)
	CreatePlaygroundProvider(client *cliClient.GrpcClient) cliClient.Provider
	GenerateAuthURL(authPort int) string
	GetServices(ctx context.Context, projectName string, provider cliClient.Provider) ([]deployment_info.Service, error)
	InteractiveLoginMCP(ctx context.Context, client *cliClient.GrpcClient, cluster string, mcpClient string) error
	ListConfig(ctx context.Context, provider cliClient.Provider, projectName string) (*defangv1.Secrets, error)
	LoadProject(ctx context.Context, loader cliClient.Loader) (*compose.Project, error)
	LoadProjectNameWithFallback(ctx context.Context, loader cliClient.Loader, provider cliClient.Provider) (string, error)
	NewProvider(ctx context.Context, providerId cliClient.ProviderID, client cliClient.FabricClient, stack string) cliClient.Provider
	OpenBrowser(url string) error
	PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) string
	RunEstimate(ctx context.Context, project *compose.Project, client *cliClient.GrpcClient, provider cliClient.Provider, providerId cliClient.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error)
	Tail(ctx context.Context, provider cliClient.Provider, project *compose.Project, options cliTypes.TailOptions) error
}
