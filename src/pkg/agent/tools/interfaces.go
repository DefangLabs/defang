// Package tools: consolidated interfaces for mocking CLI/tool dependencies
package tools

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type CLIInterface interface {
	CanIUseProvider(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, projectName string, serviceCount int) error
	ComposeDown(ctx context.Context, projectName string, fabric *client.GrpcClient, provider client.Provider) (string, error)
	ComposeUp(ctx context.Context, fabric *client.GrpcClient, provider client.Provider, stack *stacks.Parameters, params cli.ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error)
	ConfigDelete(ctx context.Context, projectName string, provider client.Provider, name string) error
	ConfigSet(ctx context.Context, projectName string, provider client.Provider, name, value string) error
	Connect(ctx context.Context, cluster string) (*client.GrpcClient, error)
	CreatePlaygroundProvider(fabric *client.GrpcClient) client.Provider
	GenerateAuthURL(authPort int) string
	GetServices(ctx context.Context, projectName string, provider client.Provider) ([]cli.ServiceEndpoint, error)
	InteractiveLoginMCP(ctx context.Context, cluster string, mcpClient string) error
	ListConfig(ctx context.Context, provider client.Provider, projectName string) (*defangv1.Secrets, error)
	LoadProject(ctx context.Context, loader client.Loader) (*compose.Project, error)
	LoadProjectNameWithFallback(ctx context.Context, loader client.Loader, provider client.Provider) (string, error)
	NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider
	PrintEstimate(mode modes.Mode, estimate *defangv1.EstimateResponse) string
	RunEstimate(ctx context.Context, project *compose.Project, fabric *client.GrpcClient, provider client.Provider, providerId client.ProviderID, region string, mode modes.Mode) (*defangv1.EstimateResponse, error)
	Tail(ctx context.Context, provider client.Provider, projectName string, options cli.TailOptions) error
}
