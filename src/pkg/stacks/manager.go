package stacks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Manager interface {
	List(ctx context.Context) ([]StackListItem, error)
	Load(name string) (*StackParameters, error)
	Create(params StackParameters) (string, error)
}

type DeploymentLister interface {
	ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
}

type manager struct {
	fabric           DeploymentLister
	targetDirectory  string
	projectName      string
	outside          bool
	workingDirectory string
}

func NewManager(fabric DeploymentLister, targetDirectory string, projectName string) (*manager, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	var outside bool
	var absTargetDirectory string
	if targetDirectory == "" {
		outside = true
		absTargetDirectory = ""
	} else {
		// abs path for targetDirectory
		var err error
		absTargetDirectory, err = filepath.Abs(targetDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for target directory: %w", err)
		}
		// Resolve symlinks for consistent comparison
		resolvedWorkingDirectory, err := filepath.EvalSymlinks(workingDirectory)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve symlinks in working directory: %w", err)
		}
		// For target directory, only resolve symlinks if the path exists
		resolvedTargetDirectory := absTargetDirectory
		if _, err := os.Stat(absTargetDirectory); err == nil {
			resolvedTargetDirectory, err = filepath.EvalSymlinks(absTargetDirectory)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve symlinks in target directory: %w", err)
			}
		}
		outside = resolvedWorkingDirectory != resolvedTargetDirectory
	}
	return &manager{
		fabric:           fabric,
		targetDirectory:  absTargetDirectory,
		projectName:      projectName,
		outside:          outside,
		workingDirectory: workingDirectory,
	}, nil
}

func (sm *manager) List(ctx context.Context) ([]StackListItem, error) {
	remoteStacks, err := sm.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote stacks: %w", err)
	}
	localStacks, err := sm.ListLocal()
	if err != nil {
		var outsideErr *OutsideError
		if !errors.As(err, &outsideErr) {
			return nil, fmt.Errorf("failed to list local stacks: %w", err)
		}
	}
	// Merge remote and local stacks into a single list of type StackOption,
	// prefer remote if both exist, so we can show last deployed time
	stackMap := make(map[string]StackListItem)
	for _, local := range localStacks {
		stackMap[local.Name] = StackListItem{
			Name:         local.Name,
			Provider:     local.Provider,
			Region:       local.Region,
			Mode:         local.Mode,
			AWSProfile:   local.AWSProfile,
			GCPProjectID: local.GCPProjectID,
			DeployedAt:   time.Time{}, // No deployed time for local
		}
	}
	for _, remote := range remoteStacks {
		stackMap[remote.StackParameters.Name] = StackListItem{
			Name:         remote.StackParameters.Name,
			Provider:     remote.StackParameters.Provider.String(),
			Region:       remote.StackParameters.Region,
			Mode:         remote.StackParameters.Mode.String(),
			AWSProfile:   remote.StackParameters.AWSProfile,
			GCPProjectID: remote.StackParameters.GCPProjectID,
			DeployedAt:   remote.DeployedAt.Local(),
		}
	}

	stackList := make([]StackListItem, 0, len(stackMap))
	for _, stack := range stackMap {
		stackList = append(stackList, stack)
	}
	return stackList, nil
}

func (sm *manager) ListLocal() ([]StackListItem, error) {
	if sm.outside {
		return nil, &OutsideError{TargetDirectory: sm.targetDirectory, WorkingDirectory: sm.workingDirectory}
	}
	return ListInDirectory(sm.targetDirectory)
}

type RemoteStack struct {
	StackParameters
	DeployedAt time.Time
}

func (sm *manager) ListRemote(ctx context.Context) ([]RemoteStack, error) {
	resp, err := sm.fabric.ListDeployments(ctx, &defangv1.ListDeploymentsRequest{
		Project: sm.projectName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}
	deployments := resp.GetDeployments()
	stackMap := make(map[string]RemoteStack)
	for _, deployment := range deployments {
		stackName := deployment.GetStack()
		if stackName == "" {
			stackName = "beta"
		}
		var providerID cliClient.ProviderID
		providerID.SetValue(deployment.GetProvider())
		// avoid overwriting existing entries, deployments are already sorted by deployed_at desc
		if _, exists := stackMap[stackName]; !exists {
			var deployedAt time.Time
			if ts := deployment.GetTimestamp(); ts != nil {
				deployedAt = ts.AsTime()
			}
			stackMap[stackName] = RemoteStack{
				StackParameters: StackParameters{
					Name:     stackName,
					Provider: providerID,
					Region:   deployment.GetRegion(),
				},
				DeployedAt: deployedAt,
			}
		}
	}
	stackParams := make([]RemoteStack, 0, len(stackMap))
	for _, params := range stackMap {
		stackParams = append(stackParams, params)
	}
	return stackParams, nil
}

type OutsideError struct {
	TargetDirectory  string
	WorkingDirectory string
}

func (e *OutsideError) Error() string {
	return fmt.Sprintf("operation not allowed: target directory (%s) is different from working directory (%s)", e.TargetDirectory, e.WorkingDirectory)
}

func (sm *manager) Load(name string) (*StackParameters, error) {
	if sm.outside {
		return nil, &OutsideError{TargetDirectory: sm.targetDirectory, WorkingDirectory: sm.workingDirectory}
	}
	params, err := ReadInDirectory(sm.targetDirectory, name)
	if err != nil {
		return nil, err
	}
	err = LoadInDirectory(sm.targetDirectory, name)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func (sm *manager) Create(params StackParameters) (string, error) {
	if sm.outside {
		return "", &OutsideError{TargetDirectory: sm.targetDirectory, WorkingDirectory: sm.workingDirectory}
	}
	return CreateInDirectory(sm.targetDirectory, params)
}
