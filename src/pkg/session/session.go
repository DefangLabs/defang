package session

import (
	"context"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type StacksManager interface {
	TargetDirectory() string
	GetStack(ctx context.Context, opts stacks.GetStackOpts) (*stacks.Parameters, string, error)
}

type Session struct {
	Stack    *stacks.Parameters
	Loader   client.Loader
	Provider client.Provider
}

type SessionLoaderOptions struct {
	ProviderID       client.ProviderID
	ProjectName      string
	ComposeFilePaths []string
	stacks.GetStackOpts
}

type SessionLoader struct {
	client client.FabricClient
	sm     StacksManager
	opts   SessionLoaderOptions
}

func NewSessionLoader(client client.FabricClient, maybeSm StacksManager, opts SessionLoaderOptions) *SessionLoader {
	return &SessionLoader{
		client: client,
		sm:     maybeSm,
		opts:   opts,
	}
}

func (sl *SessionLoader) LoadSession(ctx context.Context) (*Session, error) {
	stack, whence, err := sl.loadStack(ctx)
	if err != nil {
		return nil, err
	}
	// load provider with selected stack
	provider := cli.NewProvider(ctx, stack.Provider, sl.client, stack.Name)
	session := &Session{
		Stack:    stack,
		Loader:   sl.newLoader(),
		Provider: provider,
	}

	extraMsg := ""
	if stack.Provider == client.ProviderDefang {
		extraMsg = "; consider using BYOC (https://s.defang.io/byoc)"
	}
	term.Infof("Using the %q stack on %s from %s%s", stack.Name, stack.Provider, whence, extraMsg)

	printProviderMismatchWarnings(ctx, stack.Provider)
	return session, nil
}

func (sl *SessionLoader) loadStack(ctx context.Context) (*stacks.Parameters, string, error) {
	if sl.sm == nil {
		return &stacks.Parameters{
			Name:     stacks.DefaultBeta,
			Provider: sl.opts.ProviderID,
		}, "no stack manager available", nil
	}
	stack, whence, err := sl.sm.GetStack(ctx, sl.opts.GetStackOpts)
	if err != nil {
		if sl.opts.ProviderID != "" {
			whence = "--provider flag"
		}
		_, envSet := os.LookupEnv("DEFANG_PROVIDER")
		if envSet {
			whence = "DEFANG_PROVIDER"
		}
		if whence == "" {
			whence = "fallback stack"
		}
		return &stacks.Parameters{
			Name:     stacks.DefaultBeta,
			Provider: sl.opts.ProviderID,
		}, whence, nil
	}
	if err := stacks.LoadStackEnv(*stack, true); err != nil {
		return nil, whence, fmt.Errorf("failed to load stack env: %w", err)
	}
	return stack, whence, nil
}

func (sl *SessionLoader) newLoader() client.Loader {
	return compose.NewLoader(
		compose.WithProjectName(sl.opts.ProjectName),
		compose.WithPath(sl.opts.ComposeFilePaths...),
	)
}

func printProviderMismatchWarnings(ctx context.Context, provider client.ProviderID) {
	if provider == client.ProviderDefang {
		// Ignore any env vars when explicitly using the Defang playground provider
		// Defaults to defang provider in non-interactive mode
		if env := pkg.AwsInEnv(); env != "" {
			term.Warnf("AWS environment variables were detected (%v); did you forget --provider=aws or DEFANG_PROVIDER=aws?", env)
		}
		if env := pkg.DoInEnv(); env != "" {
			term.Warnf("DigitalOcean environment variable was detected (%v); did you forget --provider=digitalocean or DEFANG_PROVIDER=digitalocean?", env)
		}
		if env := pkg.GcpInEnv(); env != "" {
			term.Warnf("GCP project environment variable was detected (%v); did you forget --provider=gcp or DEFANG_PROVIDER=gcp?", env)
		}
	}

	switch provider {
	case client.ProviderAWS:
		if !awsInConfig(ctx) {
			term.Warn("AWS provider was selected, but AWS environment is not set")
		}
	case client.ProviderDO:
		if env := pkg.DoInEnv(); env == "" {
			term.Warn("DigitalOcean provider was selected, but DIGITALOCEAN_TOKEN environment variable is not set")
		}
	case client.ProviderGCP:
		if env := pkg.GcpInEnv(); env == "" {
			term.Warnf("GCP provider was selected, but no GCP project environment variable is set (%v)", pkg.GCPProjectEnvVars)
		}
	}
}

func awsInConfig(ctx context.Context) bool {
	_, err := aws.LoadDefaultConfig(ctx, aws.Region(""))
	return err == nil
}
