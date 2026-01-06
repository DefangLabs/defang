package stacks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Manager interface {
	List(ctx context.Context) ([]StackListItem, error)
	Load(name string) (*StackParameters, error)
	LoadParameters(params map[string]string, overload bool) error
	Create(params StackParameters) (string, error)
}

type DeploymentLister interface {
	ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error)
}

type manager struct {
	fabric          DeploymentLister
	targetDirectory string
	projectName     string
	outside         bool
}

func NewManager(fabric DeploymentLister, targetDirectory string, projectName string) (*manager, error) {
	var outside bool
	absTargetDirectory := ""
	if targetDirectory == "" {
		outside = true
	} else {
		outside = false
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
		outside:         outside,
	}, nil
}

func (sm *manager) List(ctx context.Context) ([]StackListItem, error) {
	remoteStacks, err := sm.ListRemote(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote stacks: %w", err)
	}
	localStacks, err := sm.ListLocal()
	if err != nil {
		var outsideErr *ErrOutside
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
	// sort stacks by name asc
	// sort stacks by name asc
	slices.SortFunc(stackList, func(a, b StackListItem) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return stackList, nil
}

func (sm *manager) ListLocal() ([]StackListItem, error) {
	if sm.outside {
		return nil, &ErrOutside{TargetDirectory: sm.targetDirectory}
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
			stackMap[stackName] = RemoteStack{
				StackParameters: StackParameters{
					Name:     stackName,
					Provider: providerID,
					Region:   deployment.GetRegion(),
					Mode:     modes.Mode(deployment.GetMode()),
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

type ErrOutside struct {
	TargetDirectory string
}

func (e *ErrOutside) Error() string {
	cwd, _ := os.Getwd()
	return fmt.Sprintf("operation not allowed: target directory (%s) is different from working directory (%s)", e.TargetDirectory, cwd)
}

func (sm *manager) Load(name string) (*StackParameters, error) {
	if sm.outside {
		return nil, &ErrOutside{TargetDirectory: sm.targetDirectory}
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

func (sm *manager) LoadParameters(params map[string]string, overload bool) error {
	return LoadParameters(params, overload)
}

func (sm *manager) Create(params StackParameters) (string, error) {
	if sm.outside {
		return "", &ErrOutside{TargetDirectory: sm.targetDirectory}
	}
	return CreateInDirectory(sm.targetDirectory, params)
}
