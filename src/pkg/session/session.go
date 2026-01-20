package session

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/bufbuild/connect-go"
)

type StacksManager interface {
	List(ctx context.Context) ([]stacks.ListItem, error)
	LoadLocal(name string) (*stacks.Parameters, error)
	GetRemote(ctx context.Context, name string) (*stacks.Parameters, error)
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
	RequireStack       bool
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
	stack, whence, err := sl.getStack(ctx)
	if err != nil {
		return nil, whence, err
	}
	if err := stacks.LoadStackEnv(*stack, true); err != nil {
		return nil, whence, fmt.Errorf("failed to load stack env: %w", err)
	}
	return stack, whence, nil
}

func (sl *SessionLoader) getStack(ctx context.Context) (*stacks.Parameters, string, error) {
	if sl.sm == nil {
		// Without stack manager, we can only get fallback stacks (from options)
		return sl.getFallbackStack(ctx)
	}
	if sl.opts.Stack != "" {
		return sl.getSpecifiedStack(ctx, sl.opts.Stack)
	}
	if sl.opts.Interactive && sl.opts.RequireStack {
		return sl.getStackInteractively(ctx)
	}

	return sl.getFallbackStack(ctx)
}

func (sl *SessionLoader) getSpecifiedStack(ctx context.Context, name string) (*stacks.Parameters, string, error) {
	whence := "--stack flag"
	_, envSet := os.LookupEnv("DEFANG_STACK")
	if envSet {
		whence = "DEFANG_STACK environment variable"
	}
	stack, err := sl.sm.LoadLocal(name)
	if err == nil {
		return stack, whence + " and local stack file", nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}
	// the stack file does not exist locally; try loading remotely
	stack, err = sl.sm.GetRemote(ctx, name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load stack %q remotely: %w", name, err)
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

func (sl *SessionLoader) getStackInteractively(ctx context.Context) (*stacks.Parameters, string, error) {
	stackSelector := stacks.NewSelector(sl.ec, sl.sm)
	stack, err := stackSelector.SelectStack(ctx, stacks.SelectStackOptions{
		AllowCreate: sl.opts.AllowStackCreation,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to select stack: %w", err)
	}
	return stack, "interactive selection", nil
}

func (sl *SessionLoader) getFallbackStack(ctx context.Context) (*stacks.Parameters, string, error) {
	var params *stacks.Parameters
	var whence string
	// Check Fabric for default stack (set by Portal or CLI); this requires the project name
	projectName, projectLoaded, err := sl.newLoader().LoadProjectName(ctx)
	if err != nil {
		term.Debugf("Could not load project name; using default: %v", err)
	} else {
		res, err := sl.client.GetDefaultStack(ctx, &defangv1.GetDefaultStackRequest{
			Project: projectName,
		})
		if err != nil {
			if connect.CodeOf(err) != connect.CodeNotFound {
				return nil, "", err
			}
			term.Debugf("No default stack set for project %q; using default", projectName)
		} else {
			whence = "default stack from server"
			params, err = stacks.NewParametersFromContent(res.Stack.Name, res.Stack.StackFile)
			// A default stack may not change the Compose project name or file paths, because we got those from the Compose file
			if pn, ok := params.Variables["COMPOSE_PROJECT_NAME"]; ok && pn != projectName {
				term.Warnf("Using default stack %q for project %q, but the stack specifies COMPOSE_PROJECT_NAME=%q", res.Stack.Name, projectName, pn)
			}
			if cf, ok := params.Variables["COMPOSE_FILE"]; ok && projectLoaded {
				term.Warnf("Using default stack %q for project %q, but the stack specifies COMPOSE_FILE=%q", res.Stack.Name, projectName, cf)
			}
			return params, whence, err
		}
	}

	whence = "default provider"
	if sl.opts.ProviderID != "" {
		whence = "--provider flag"
	}
	_, envSet := os.LookupEnv("DEFANG_PROVIDER")
	if envSet {
		whence = "DEFANG_PROVIDER"
	}
	params = &stacks.Parameters{
		Name:     stacks.DefaultBeta,
		Provider: sl.opts.ProviderID,
	}
	return params, whence, nil
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
