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

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type DeploymentLister interface {
	ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
}

type manager struct {
	fabric          DeploymentLister
	targetDirectory string
	projectName     string
}

func NewManager(fabric DeploymentLister, targetDirectory string, projectName string) (*manager, error) {
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

func (sm *manager) List(ctx context.Context) ([]StackListItem, error) {
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
	stackMap := make(map[string]StackListItem)
	for _, remote := range remoteStacks {
		stackMap[remote.Name] = remote
	}
	for _, local := range localStacks {
		remote, exists := stackMap[local.StackParameters.Name]
		if exists {
			local.DeployedAt = remote.DeployedAt
			stackMap[local.StackParameters.Name] = local
		} else {
			stackMap[local.StackParameters.Name] = StackListItem{
				StackParameters: local.StackParameters,
			}
		}
	}

	stackList := make([]StackListItem, 0, len(stackMap))
	for _, stack := range stackMap {
		stackList = append(stackList, stack)
	}

	// sort stacks by name asc
	slices.SortFunc(stackList, func(a, b StackListItem) int {
		return strings.Compare(a.Name, b.Name)
	})

	return stackList, nil
}

func (sm *manager) ListLocal() ([]StackListItem, error) {
	return ListInDirectory(sm.targetDirectory)
}

func (sm *manager) ListRemote(ctx context.Context) ([]StackListItem, error) {
	resp, err := sm.fabric.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: sm.projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	stackMap := make(map[string]StackListItem)
	for _, deployment := range resp.GetDeployments() {
		stackName := deployment.GetStack()
		if stackName == "" {
			stackName = DefaultBeta
		}
		var providerID client.ProviderID
		providerID.SetValue(deployment.GetProvider())
		// avoid overwriting existing entries, deployments are already sorted by deployed_at desc
		if _, exists := stackMap[stackName]; !exists {
			var deployedAt time.Time
			if ts := deployment.GetTimestamp(); ts != nil {
				deployedAt = ts.AsTime()
			}
			variables := map[string]string{
				"DEFANG_PROVIDER": providerID.String(),
				"DEFANG_MODE":     deployment.GetMode().String(),
			}
			regionVarName := client.GetRegionVarName(providerID)
			region := deployment.GetRegion()
			if region != "" {
				variables[regionVarName] = region
			}
			params, err := ParamsFromMap(variables)
			if err != nil {
				term.Warnf("Skipping invalid remote deployment %s: %v\n", stackName, err)
				continue
			}
			params.Name = stackName
			stackMap[stackName] = StackListItem{
				StackParameters: params,
				DeployedAt:      deployedAt,
			}
		}
	}
	stackParams := make([]StackListItem, 0, len(stackMap))
	for _, params := range stackMap {
		stackParams = append(stackParams, params)
	}

	// sort by deployed at desc
	slices.SortFunc(stackParams, func(a, b StackListItem) int {
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

func (sm *manager) Load(ctx context.Context, name string) (*StackParameters, error) {
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

func (sm *manager) LoadLocal(name string) (*StackParameters, error) {
	params, err := ReadInDirectory(sm.targetDirectory, name)
	if err != nil {
		return nil, err
	}
	err = sm.LoadParameters(*params, false)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func (sm *manager) LoadRemote(ctx context.Context, name string) (*StackParameters, error) {
	remoteStacks, err := sm.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote stacks: %w", err)
	}
	var remoteStack *StackListItem
	for i := range remoteStacks {
		if remoteStacks[i].Name == name {
			remoteStack = &remoteStacks[i]
			break
		}
	}
	if remoteStack == nil {
		return nil, fmt.Errorf("unable to find stack %q", name)
	}
	err = sm.LoadParameters(remoteStack.StackParameters, false)
	if err != nil {
		return nil, fmt.Errorf("unable to import stack %q: %w", name, err)
	}

	return &remoteStack.StackParameters, nil
}

func (sm *manager) LoadParameters(params StackParameters, overload bool) error {
	return LoadParameters(params, overload)
}

func (sm *manager) Create(params StackParameters) (string, error) {
	if sm.targetDirectory == "" {
		return "", &ErrOutside{Operation: "Create", TargetDirectory: sm.targetDirectory}
	}
	return CreateInDirectory(sm.targetDirectory, params)
}
