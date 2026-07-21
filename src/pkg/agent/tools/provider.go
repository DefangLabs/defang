package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg/agent/common"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

const CreateNewStack = "Create new stack"

type ProviderCreator interface {
	NewProvider(ctx context.Context, providerId client.ProviderID, client client.FabricClient, stack string) client.Provider
}

type providerPreparer struct {
	pc ProviderCreator
	ec elicitations.Controller
	fc client.FabricClient
	sm stacks.Manager
}

func NewProviderPreparer(pc ProviderCreator, ec elicitations.Controller, fc client.FabricClient, sm stacks.Manager) *providerPreparer {
	return &providerPreparer{
		pc: pc,
		ec: ec,
		fc: fc,
		sm: sm,
	}
}

func (pp *providerPreparer) SetupProvider(ctx context.Context, stack *stacks.Parameters) (*client.ProviderID, client.Provider, error) {
	if stack.Name == "" && pp.ec.IsSupported() {
		selector := stacks.NewSelector(pp.ec, pp.sm, pp.fc)
		newStack, err := selector.SelectStack(ctx, stacks.SelectStackOptions{
			AllowStackCreation: true,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to setup stack: %w", err)
		}
		*stack = *newStack
		err = stacks.LoadStackEnv(*stack, false)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load stack env: %w", err)
		}
	}

	term.Debug("Function invoked: cli.NewProvider")
	provider := pp.pc.NewProvider(ctx, stack.Provider, pp.fc, stack.Name)
	if err := provider.Authenticate(ctx, pp.ec.IsSupported()); err != nil {
		return nil, nil, fmt.Errorf("failed to authenticate with provider %q: %w", stack.Provider, err)
	}
	providerID := stack.Provider
	return &providerID, provider, nil
}

type projectWorkingDirResolver interface {
	ResolveProjectWorkingDir(context.Context) (string, error)
}

func setupProviderAndLoader(ctx context.Context, loader client.Loader, params common.LoaderParams, cli CLIInterface, ec elicitations.Controller, fabric *client.GrpcClient, sc StackConfig) (client.Provider, client.Loader, error) {
	resolver, ok := loader.(projectWorkingDirResolver)
	if !ok {
		return nil, nil, fmt.Errorf("loader %T does not support resolving the project working directory", loader)
	}
	projectWorkingDir, err := resolver.ResolveProjectWorkingDir(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get project working directory: %w", err)
	}

	sm, err := stacks.NewManager(fabric, projectWorkingDir, params.ProjectName, ec)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create stack manager: %w", err)
	}

	initialProvider, initialStack := sc.Stack.Provider, sc.Stack.Name
	pp := NewProviderPreparer(cli, ec, fabric, sm)
	_, provider, err := pp.SetupProvider(ctx, sc.Stack)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to setup provider: %w", err)
	}

	if sc.Stack.Provider != initialProvider || sc.Stack.Name != initialStack {
		// ConfigureAgentLoader has already changed into params.WorkingDirectory.
		// Keep that absolute base so relative ComposeFilePaths resolve exactly
		// as they did for the incoming loader.
		loaderBaseDir, err := os.Getwd()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get loader base directory: %w", err)
		}
		params.WorkingDirectory = loaderBaseDir
		loader, err = common.ConfigureAgentLoader(params, sc.Stack)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to configure loader for selected stack: %w", err)
		}
	}

	return provider, loader, nil
}
