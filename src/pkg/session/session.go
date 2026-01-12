package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
)

type Session struct {
	Stack    *stacks.StackParameters
	Loader   client.Loader
	Provider client.Provider
}

type SessionLoaderOptions struct {
	Stack              string
	ProviderID         client.ProviderID
	ProjectName        string
	ComposeFilePaths   []string
	AllowStackCreation bool
	Interactive        bool
}

type SessionLoader struct {
	client client.FabricClient
	ec     elicitations.Controller
	opts   SessionLoaderOptions
}

func NewSessionLoader(client client.FabricClient, ec elicitations.Controller, opts SessionLoaderOptions) *SessionLoader {
	return &SessionLoader{
		client: client,
		ec:     ec,
		opts:   opts,
	}
}

func (sl *SessionLoader) LoadSession(ctx context.Context) (*Session, error) {
	// cd into working dir with .defang directory, assume a compose file is also there
	targetDirectory, err := sl.findTargetDirectory()
	if err != nil {
		if sl.opts.ProjectName == "" {
			if errors.Is(err, os.ErrNotExist) {
				return nil, errors.New("project name is required when outside of a project directory")
			}
			return nil, err
		}
	}

	// load stack
	stack, whence, err := sl.loadStack(ctx, targetDirectory)
	if err != nil {
		return nil, err
	}
	// TODO: update the environment and globals with the values from any corresponding stack parameters unless overwritten by flags
	// iterate over the stack variables
	// for each, if the corresponding global.ToMap() is not the empty value, bail
	// if any global config properties
	// TODO: the stack may change the project name and compose file paths
	// if stack.ProjectName != "" {
	//   sl.opts.ProjectName = stack.ProjectName
	// }
	// if len(stack.ComposeFilePaths) > 0 {
	//   sl.opts.ComposeFilePaths = stack.ComposeFilePaths
	// }
	// initialize loader
	loader := compose.NewLoader(
		compose.WithProjectName(sl.opts.ProjectName),
		compose.WithPath(sl.opts.ComposeFilePaths...),
	)
	// load provider with selected stack
	provider := cli.NewProvider(ctx, sl.opts.ProviderID, sl.client, stack.Name)
	session := &Session{
		Stack:    stack,
		Loader:   loader,
		Provider: provider,
	}
	extraMsg := ""
	if stack.Provider == client.ProviderDefang {
		extraMsg = "; consider using BYOC (https://s.defang.io/byoc)"
	}
	term.Infof("Using the %q stack on %s from %s%s", stack.Name, stack.Provider, whence, extraMsg)

	printProviderMismatchWarnings(ctx, stack.Provider)
	_, err = provider.AccountInfo(ctx)
	if err != nil {
		// HACK: return the session even on error to allow `whoami` and `compose config` to return a result even on provider error
		return session, fmt.Errorf("failed to get account info from provider %q: %w", stack.Provider, err)
	}
	// also call accountInfo and update the region
	return session, nil
}

func (sl *SessionLoader) findTargetDirectory() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	for {
		info, err := os.Stat(filepath.Join(wd, stacks.Directory))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("failed to stat .defang directory: %w", err)
			}
		} else if info.IsDir() {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			// reached root directory
			return "", os.ErrNotExist
		}
		wd = parent
	}
}

func (sl *SessionLoader) loadStack(ctx context.Context, targetDirectory string) (*stacks.StackParameters, string, error) {
	sm, err := stacks.NewManager(sl.client, targetDirectory, sl.opts.ProjectName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create stack manager: %w", err)
	}
	if sl.opts.Stack != "" {
		return sl.loadSpecifiedStack(ctx, sm, sl.opts.Stack)
	}
	if sl.opts.Interactive {
		stackSelector := stacks.NewSelector(sl.ec, sm)
		stack, err := stackSelector.SelectStackWithOptions(ctx, stacks.SelectStackOptions{
			AllowCreate: sl.opts.AllowStackCreation,
			AllowImport: sm.TargetDirectory() == "",
		})

		if err != nil {
			return nil, "", fmt.Errorf("failed to select stack: %w", err)
		}
		return stack, "interactive selection", nil
	}

	return sl.loadFallbackStack()
}

func (sl *SessionLoader) loadSpecifiedStack(ctx context.Context, sm stacks.Manager, name string) (*stacks.StackParameters, string, error) {
	whence := "--stack flag"
	_, envSet := os.LookupEnv("DEFANG_STACK")
	if envSet {
		whence = "DEFANG_STACK environment variable"
	}
	stack, err := sm.LoadLocal(name)
	if err == nil {
		return stack, whence + " and local stack file", nil
	}
	// the stack file does not exist locally
	if !os.IsNotExist(err) {
		return nil, "", err
	}
	stack, err = sm.LoadRemote(ctx, name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack %q remotely: %w", name, err)
	}
	// persist the remote stack file to the local target directory
	stackFilename, err := sm.Create(*stack)
	if err != nil && !errors.Is(err, &stacks.ErrOutside{}) {
		return nil, "", fmt.Errorf("failed to save imported stack %q to local directory: %w", name, err)
	}
	if stackFilename != "" {
		term.Infof("Stack %q loaded and saved to %q. Add this file to source control", name, stackFilename)
	}
	return stack, whence + " and previous deployment", nil
}

func (sl *SessionLoader) loadFallbackStack() (*stacks.StackParameters, string, error) {
	whence := "--provider flag"
	_, envSet := os.LookupEnv("DEFANG_PROVIDER")
	if envSet {
		whence = "DEFANG_PROVIDER environment variable and previous deployment"
	}
	if sl.opts.ProviderID == "" {
		return nil, "", errors.New("--provider is required if --stack is not specified")
	}
	// TODO: list remote stacks, and if there is exactly one with the matched provider, load it
	return &stacks.StackParameters{
		Name: stacks.DefaultBeta,
		Variables: map[string]string{
			"DEFANG_PROVIDER": sl.opts.ProviderID.String(),
		},
	}, whence, nil
}

func printProviderMismatchWarnings(ctx context.Context, provider client.ProviderID) {
	if provider == client.ProviderDefang {
		// Ignore any env vars when explicitly using the Defang playground provider
		// Defaults to defang provider in non-interactive mode
		if pkg.AwsInEnv() {
			term.Warn("AWS environment variables were detected; did you forget --provider=aws or DEFANG_PROVIDER=aws?")
		}
		if pkg.DoInEnv() {
			term.Warn("DIGITALOCEAN_TOKEN environment variable was detected; did you forget --provider=digitalocean or DEFANG_PROVIDER=digitalocean?")
		}
		if pkg.GcpInEnv() {
			term.Warn("GCP_PROJECT_ID/CLOUDSDK_CORE_PROJECT environment variable was detected; did you forget --provider=gcp or DEFANG_PROVIDER=gcp?")
		}
	}

	switch provider {
	case client.ProviderAWS:
		if !awsInConfig(ctx) {
			term.Warn("AWS provider was selected, but AWS environment is not set")
		}
	case client.ProviderDO:
		if !pkg.DoInEnv() {
			term.Warn("DigitalOcean provider was selected, but DIGITALOCEAN_TOKEN environment variable is not set")
		}
	case client.ProviderGCP:
		if !pkg.GcpInEnv() {
			term.Warn("GCP provider was selected, but GCP_PROJECT_ID environment variable is not set")
		}
	}
}

func awsInConfig(ctx context.Context) bool {
	_, err := aws.LoadDefaultConfig(ctx, aws.Region(""))
	return err == nil
}
