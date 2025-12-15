package stacks

import (
	"context"
	"fmt"
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
	fabric          DeploymentLister
	targetDirectory string
	projectName     string
}

func NewManager(fabric DeploymentLister, targetDirectory string, projectName string) *manager {
	return &manager{
		fabric:          fabric,
		targetDirectory: targetDirectory,
		projectName:     projectName,
	}
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
			DeployedAt:   remote.DeployedAt,
		}
	}

	stackList := make([]StackListItem, 0, len(stackMap))
	for _, stack := range stackMap {
		stackList = append(stackList, stack)
	}
	return stackList, nil
}

func (sm *manager) ListLocal() ([]StackListItem, error) {
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

func (sm *manager) Load(name string) (*StackParameters, error) {
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
	return CreateInDirectory(sm.targetDirectory, params)
}
