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

	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/timeutils"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Lister interface {
	ListStacks(ctx context.Context, req *defangv1.ListStacksRequest) (*defangv1.ListStacksResponse, error)
}

type manager struct {
	fabric          Lister
	targetDirectory string
	projectName     string
}

func NewManager(fabric Lister, targetDirectory string, projectName string) (*manager, error) {
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
	// prefer local if both exist, but keep remote deployed time if available
	stackMap := make(map[string]ListItem)
	for _, remote := range remoteStacks {
		stackMap[remote.Name] = remote
	}
	for _, local := range localStacks {
		remote, exists := stackMap[local.Parameters.Name]
		if exists {
			local.DeployedAt = remote.DeployedAt
			stackMap[local.Parameters.Name] = local
		} else {
			stackMap[local.Parameters.Name] = ListItem{
				Parameters: local.Parameters,
			}
		}
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
		name := stack.GetName()
		if name == "" {
			name = DefaultBeta
		}
		bytes := stack.GetStackFile()
		params, err := NewParametersFromContent(name, bytes)
		if err != nil {
			term.Warnf("Skipping invalid remote stack %s: %v\n", name, err)
			continue
		}
		stackParams = append(stackParams, ListItem{
			Parameters: *params,
			DeployedAt: timeutils.AsTime(stack.GetLastDeployedAt(), time.Time{}),
		})
	}

	// sort by deployed at desc
	slices.SortFunc(stackParams, func(a, b ListItem) int {
		return b.DeployedAt.Compare(a.DeployedAt)
	})
	return stackParams, nil
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
		if errors.Is(err, os.ErrNotExist) {
			term.Infof("stack file not found, attempting to import from previous deployments: %v", err)
			return sm.LoadRemote(ctx, name)
		}
		return nil, err
	}
	return params, nil
}

func (sm *manager) LoadLocal(name string) (*Parameters, error) {
	params, err := ReadInDirectory(sm.targetDirectory, name)
	if err != nil {
		return nil, err
	}
	err = sm.LoadStackEnv(*params, false)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func (sm *manager) LoadRemote(ctx context.Context, name string) (*Parameters, error) {
	remoteStacks, err := sm.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote stacks: %w", err)
	}
	var remoteStack *ListItem
	for i := range remoteStacks {
		if remoteStacks[i].Name == name {
			remoteStack = &remoteStacks[i]
			break
		}
	}
	if remoteStack == nil {
		return nil, fmt.Errorf("unable to find stack %q", name)
	}
	err = sm.LoadStackEnv(remoteStack.Parameters, false)
	if err != nil {
		return nil, fmt.Errorf("unable to import stack %q: %w", name, err)
	}

	return &remoteStack.Parameters, nil
}

func (sm *manager) LoadStackEnv(params Parameters, overload bool) error {
	return LoadStackEnv(params, overload)
}

func (sm *manager) Create(params Parameters) (string, error) {
	if sm.targetDirectory == "" {
		return "", &ErrOutside{Operation: "Create", TargetDirectory: sm.targetDirectory}
	}
	return CreateInDirectory(sm.targetDirectory, params)
}
