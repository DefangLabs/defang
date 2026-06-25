package stacks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Lister interface {
	GetDefaultStack(ctx context.Context, req *defangv1.GetDefaultStackRequest) (*defangv1.GetStackResponse, error)
	GetStack(ctx context.Context, req *defangv1.GetStackRequest) (*defangv1.GetStackResponse, error)
	ListStacks(ctx context.Context, req *defangv1.ListStacksRequest) (*defangv1.ListStacksResponse, error)
	ListRecipes(ctx context.Context) (*defangv1.ListRecipesResponse, error)
}

type manager struct {
	ec              elicitations.Controller
	fabric          Lister
	targetDirectory string
	projectName     string
}

func NewManager(fabric Lister, targetDirectory string, projectName string, ec elicitations.Controller) (*manager, error) {
	absTargetDirectory := ""
	if targetDirectory != "" {
		// abs path for targetDirectory
		var err error
		absTargetDirectory, err = filepath.Abs(targetDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for target directory: %w", err)
		}
	}
	return &manager{
		fabric:          fabric,
		targetDirectory: absTargetDirectory,
		projectName:     projectName,
		ec:              ec,
	}, nil
}

func (sm *manager) TargetDirectory() string {
	return sm.targetDirectory
}

func (sm *manager) List(ctx context.Context) ([]ListItem, error) {
	remoteStacks, err := sm.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote stacks: %w", err)
	}
	localStacks, err := sm.ListLocal()
	if err != nil {
		return nil, fmt.Errorf("failed to list local stacks: %w", err)
	}
	// Merge remote and local stacks into a single list of type StackOption,
	// prefer remote if both exist
	stackMap := make(map[string]ListItem)
	for _, local := range localStacks {
		stackMap[local.Name] = local
	}
	for _, remote := range remoteStacks {
		stackMap[remote.Name] = remote
	}

	stackList := make([]ListItem, 0, len(stackMap))
	for _, stack := range stackMap {
		stackList = append(stackList, stack)
	}

	// sort stacks by name asc
	slices.SortFunc(stackList, func(a, b ListItem) int {
		return strings.Compare(a.Name, b.Name)
	})

	return stackList, nil
}

func (sm *manager) ListLocal() ([]ListItem, error) {
	return ListInDirectory(sm.targetDirectory)
}

func (sm *manager) ListRemote(ctx context.Context) ([]ListItem, error) {
	if sm.projectName == "" {
		return nil, errors.New("project name is required to list remote stacks")
	}
	resp, err := sm.fabric.ListStacks(ctx, &defangv1.ListStacksRequest{
		Project: sm.projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list stacks: %w", err)
	}
	stackParams := make([]ListItem, 0, len(resp.GetStacks()))
	for _, stack := range resp.GetStacks() {
		params, err := newParametersFromPB(stack)
		if err != nil {
			term.Warnf("Skipping invalid remote stack %s: %v\n", stack.GetName(), err)
			continue
		}
		account := params.Account()
		if account == "" {
			account = stack.GetProviderAccountId()
		}
		deployedAt := timeutils.AsTime(stack.GetLastDeployedAt(), time.Time{})
		if !deployedAt.IsZero() {
			deployedAt = deployedAt.Local()
		}
		stackParams = append(stackParams, ListItem{
			Name:       params.Name,
			Provider:   params.Provider,
			Mode:       params.Mode,
			Region:     params.Region,
			Account:    account,
			DeployedAt: deployedAt,
			Default:    stack.GetIsDefault(),
		})
	}

	// sort by deployed at desc
	slices.SortFunc(stackParams, func(a, b ListItem) int {
		return b.DeployedAt.Compare(a.DeployedAt)
	})
	return stackParams, nil
}

func newParametersFromPB(stack *defangv1.Stack) (*Parameters, error) {
	name := stack.GetName()
	if name == "" {
		name = DefaultBeta
	}
	bytes := stack.GetStackFile()
	params, err := NewParametersFromContent(name, bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote stack content: %w", err)
	}
	// fill in missing fields from remote stack info
	if params.Mode == modes.RecipeUnspecified {
		params.Mode = modes.FromMode(stack.GetMode())
	}
	if params.Region == "" {
		params.Region = stack.GetRegion()
	}
	if params.Provider == "" {
		params.Provider.SetValue(stack.GetProvider())
	}
	return params, nil
}

type ErrOutside struct {
	Operation       string
	TargetDirectory string
}

func (e *ErrOutside) Error() string {
	cwd, _ := os.Getwd()
	return fmt.Sprintf("%s not allowed: target directory (%s) is different from working directory (%s)", e.Operation, e.TargetDirectory, cwd)
}

func (sm *manager) Load(ctx context.Context, name string) (*Parameters, error) {
	params, err := sm.LoadLocal(name)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		// Local stack file not found, attempting import from previous deployment
		var err2 error
		params, err2 = sm.GetRemote(ctx, name)
		if err2 != nil {
			// Error: could not load stack parameters: stack "beta" does not exist for project "foobar"; open .defang/beta: no such file or directory
			return nil, fmt.Errorf("%w; %w", err2, err)
		}
		term.Info("Stack file imported from previous deployment")
	}
	return params, nil
}

func (sm *manager) LoadLocal(name string) (*Parameters, error) {
	params, err := ReadInDirectory(sm.targetDirectory, name)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func (sm *manager) GetRemote(ctx context.Context, name string) (*Parameters, error) {
	if sm.projectName == "" {
		return nil, errors.New("project name is required to get remote stack")
	}
	if name == "" {
		return nil, errors.New("stack name is required to get remote stack")
	}
	remoteStack, err := sm.fabric.GetStack(ctx, &defangv1.GetStackRequest{
		Project: sm.projectName,
		Stack:   name,
	})
	if err != nil {
		if connect.CodeOf(err) == connect.CodeNotFound {
			return nil, &ErrNotExist{ProjectName: sm.projectName, StackName: name}
		}
		return nil, fmt.Errorf("failed to get remote stack: %w", err)
	}

	return newParametersFromPB(remoteStack.GetStack())
}

func (sm *manager) Create(params Parameters) (string, error) {
	if sm.targetDirectory == "" {
		return "", &ErrOutside{Operation: "Create", TargetDirectory: sm.targetDirectory}
	}
	return CreateInDirectory(sm.targetDirectory, params)
}

type GetStackOpts struct {
	Default     Parameters
	Interactive bool
	SelectStackOptions
}

func GetFallbackStack(defaults Parameters) (*Parameters, string, error) {
	if defaults.Provider != client.ProviderAuto && defaults.Provider != "" {
		whence := "DEFANG_PROVIDER"
		var fromEnv client.ProviderID
		if err := fromEnv.Set(os.Getenv("DEFANG_PROVIDER")); err == nil && fromEnv != defaults.Provider {
			whence = "--provider flag"
		}
		defaults.Name = DefaultBeta
		return &defaults, whence, nil
	}
	return nil, "", errors.New("no provider specified")
}

func (sm *manager) GetStack(ctx context.Context, opts GetStackOpts) (*Parameters, string, error) {
	// use --stack if available
	if opts.Default.Name != "" {
		return sm.getSpecifiedStack(ctx, opts.Default.Name) // TODO: merge with opts.Default?
	}
	// use legacy --provider if available
	if fallback, whence, err := GetFallbackStack(opts.Default); err == nil {
		return fallback, whence, nil
	}
	// fallback to interactive
	if opts.Interactive {
		return sm.getStackInteractively(ctx, opts) // TODO: merge with opts.Default?
	}
	// fallback to default stack for project
	stack, whence, err := sm.getDefaultStack(ctx)
	if err != nil {
		var defaultStackNotSet *ErrDefaultStackNotSet
		if !errors.As(err, &defaultStackNotSet) {
			return nil, "", err
		}
		// no default stack for project; use fallback
		whence := "fallback stack"
		fallback := opts.Default
		fallback.Name = DefaultBeta
		return &fallback, whence, nil
	}

	return stack, whence, nil
}

type ErrNotExist struct {
	ProjectName string
	StackName   string
}

func (e *ErrNotExist) Error() string {
	return fmt.Sprintf("stack %q does not exist for project %q", e.StackName, e.ProjectName)
}

type ErrDefaultStackNotSet struct {
	ProjectName string
}

func (e *ErrDefaultStackNotSet) Error() string {
	return fmt.Sprintf("no default stack set for project %q", e.ProjectName)
}

func (sm *manager) getSpecifiedStack(ctx context.Context, name string) (*Parameters, string, error) {
	whence := "--stack flag"
	_, envSet := os.LookupEnv("DEFANG_STACK")
	if envSet {
		whence = "DEFANG_STACK environment variable"
	}
	stack, err := sm.LoadLocal(name)
	if err == nil {
		return stack, whence + " and local stack file", nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}
	// the stack file does not exist locally; try loading remotely
	stack, err = sm.GetRemote(ctx, name)
	if err != nil {
		return nil, "", err
	}
	// persist the remote stack file to the local target directory
	stackFilename, err := sm.Create(*stack)
	var errOutside *ErrOutside
	if err != nil && !errors.As(err, &errOutside) {
		return nil, "", fmt.Errorf("failed to save imported stack %q to local directory: %w", name, err)
	}
	if stackFilename != "" {
		term.Infof("Stack %q loaded and saved to %q. Add this file to source control.", name, stackFilename)
	}
	return stack, whence + " and previous deployment", nil
}

func (sm *manager) getStackInteractively(ctx context.Context, opts GetStackOpts) (*Parameters, string, error) {
	stackSelector := NewSelector(sm.ec, sm, sm.fabric)
	// TODO: pass fallback stack to selector wizard for pre-filling
	stack, err := stackSelector.SelectStack(ctx, opts.SelectStackOptions)
	if err != nil {
		return nil, "", fmt.Errorf("failed to select stack: %w", err)
	}
	return stack, "interactive selection", nil
}

func (sm *manager) getDefaultStack(ctx context.Context) (*Parameters, string, error) {
	// Check Fabric for default stack (set by Portal or CLI); this requires the project name
	if sm.projectName == "" {
		return nil, "", errors.New("project name is required to load default stack")
	}

	res, err := sm.fabric.GetDefaultStack(ctx, &defangv1.GetDefaultStackRequest{
		Project: sm.projectName,
	})
	if err != nil {
		if connect.CodeOf(err) != connect.CodeNotFound {
			return nil, "", err
		}
		return nil, "", &ErrDefaultStackNotSet{ProjectName: sm.projectName}
	}

	whence := "default stack from server"
	params, err := newParametersFromPB(res.Stack)
	if err != nil {
		return nil, whence, err
	}
	// A default stack may not change the Compose project name or file paths, because we got those from the Compose file
	if pn, ok := params.Variables["COMPOSE_PROJECT_NAME"]; ok && pn != sm.projectName {
		return nil, whence, fmt.Errorf("using default stack %q for project %q, but the stack specifies COMPOSE_PROJECT_NAME=%q", res.Stack.Name, sm.projectName, pn)
	}
	if cf, ok := params.Variables["COMPOSE_FILE"]; ok {
		term.Warnf("Using default stack %q for project %q, but the stack specifies COMPOSE_FILE=%q", res.Stack.Name, sm.projectName, cf)
	}
	return params, whence, nil
}
