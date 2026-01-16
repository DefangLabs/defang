package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/compose-spec/compose-go/v2/consts"
)

type StacksManager interface {
	List(ctx context.Context) ([]stacks.ListItem, error)
	LoadLocal(name string) (*stacks.Parameters, error)
	LoadRemote(ctx context.Context, name string) (*stacks.Parameters, error)
	Create(params stacks.Parameters) (string, error)
	TargetDirectory() string
}

type Session struct {
	Stack    *stacks.Parameters
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
	sm     StacksManager
	opts   SessionLoaderOptions
}

func NewSessionLoader(client client.FabricClient, ec elicitations.Controller, maybeSm StacksManager, opts SessionLoaderOptions) *SessionLoader {
	return &SessionLoader{
		client: client,
		ec:     ec,
		sm:     maybeSm,
		opts:   opts,
	}
}

func (sl *SessionLoader) LoadSession(ctx context.Context) (*Session, error) {
	// load stack
	stack, whence, err := sl.loadStack(ctx)
	if err != nil {
		return nil, err
	}
	loader, err := sl.newLoader(stack)
	if err != nil {
		return nil, fmt.Errorf("failed to create loader for stack %q: %w", stack.Name, err)
	}
	// load provider with selected stack
	provider := cli.NewProvider(ctx, stack.Provider, sl.client, stack.Name)
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
	return session, nil
}

func (sl *SessionLoader) loadStack(ctx context.Context) (*stacks.Parameters, string, error) {
	if sl.sm == nil {
		// Without stack manager, we can only load fallback stacks (from options)
		return sl.loadFallbackStack()
	}
	if sl.opts.Stack != "" {
		return sl.loadSpecifiedStack(ctx, sl.opts.Stack)
	}
	if sl.opts.Interactive {
		return sl.loadStackInteractively(ctx)
	}

	return sl.loadFallbackStack()
}

func (sl *SessionLoader) loadSpecifiedStack(ctx context.Context, name string) (*stacks.Parameters, string, error) {
	whence := "--stack flag"
	_, envSet := os.LookupEnv("DEFANG_STACK")
	if envSet {
		whence = "DEFANG_STACK environment variable"
	}
	stack, err := sl.sm.LoadLocal(name)
	if err == nil {
		err = stacks.LoadStackEnv(*stack, false)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load stack env: %w", err)
		}
		return stack, whence + " and local stack file", nil
	}
	// the stack file does not exist locally
	if !os.IsNotExist(err) {
		return nil, "", err
	}
	stack, err = sl.sm.LoadRemote(ctx, name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack %q remotely: %w", name, err)
	}
	err = stacks.LoadStackEnv(*stack, false)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack env: %w", err)
	}
	// persist the remote stack file to the local target directory
	stackFilename, err := sl.sm.Create(*stack)
	var errOutside *stacks.ErrOutside
	if err != nil && !errors.As(err, &errOutside) {
		return nil, "", fmt.Errorf("failed to save imported stack %q to local directory: %w", name, err)
	}
	if stackFilename != "" {
		term.Infof("Stack %q loaded and saved to %q. Add this file to source control", name, stackFilename)
	}
	return stack, whence + " and previous deployment", nil
}

func (sl *SessionLoader) loadStackInteractively(ctx context.Context) (*stacks.Parameters, string, error) {
	stackSelector := stacks.NewSelector(sl.ec, sl.sm)
	stack, err := stackSelector.SelectStack(ctx, stacks.SelectStackOptions{
		AllowCreate: sl.opts.AllowStackCreation,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to select stack: %w", err)
	}
	err = stacks.LoadStackEnv(*stack, false)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack env: %w", err)
	}

	return stack, "interactive selection", nil
}

func (sl *SessionLoader) loadFallbackStack() (*stacks.Parameters, string, error) {
	whence := "--provider flag"
	_, envSet := os.LookupEnv("DEFANG_PROVIDER")
	if envSet {
		whence = "DEFANG_PROVIDER"
	}
	if sl.opts.ProviderID == "" || sl.opts.ProviderID == client.ProviderAuto {
		return nil, "", errors.New("--provider must be specified if --stack is not specified")
	}
	stack := &stacks.Parameters{
		Name:     stacks.DefaultBeta,
		Provider: sl.opts.ProviderID,
	}
	err := stacks.LoadStackEnv(*stack, false)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack env: %w", err)
	}
	return stack, whence, nil
}

func (sl *SessionLoader) newLoader(stack *stacks.Parameters) (client.Loader, error) {
	// the stack may change the project name and compose file paths
	if stack.Variables["COMPOSE_PROJECT_NAME"] != "" {
		sl.opts.ProjectName = stack.Variables["COMPOSE_PROJECT_NAME"]
	}
	if len(stack.Variables["COMPOSE_PATH"]) > 0 {
		paths, err := parseComposePaths(stack.Variables["COMPOSE_PATH"])
		if err != nil {
			return nil, fmt.Errorf("failed to parse COMPOSE_PATH from stack variables: %w", err)
		}
		sl.opts.ComposeFilePaths = paths
	}
	// initialize loader
	loader := compose.NewLoader(
		compose.WithProjectName(sl.opts.ProjectName),
		compose.WithPath(sl.opts.ComposeFilePaths...),
	)
	return loader, nil
}

func parseComposePaths(pathsStr string) ([]string, error) {
	if len(pathsStr) <= 0 {
		return []string{}, nil
	}
	paths := make([]string, 0)
	sep := pkg.Getenv(consts.ComposePathSeparator, string(os.PathListSeparator))
	for _, p := range strings.Split(pathsStr, sep) {
		absPath, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		paths = append(paths, absPath)
	}
	return paths, nil
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
