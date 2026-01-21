package stacks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockFabricClient implements FabricClient interface for testing
type mockFabricClient struct {
	defaultStack *defangv1.Stack
	stacks       []*defangv1.Stack
	listErr      error
}

func (m *mockFabricClient) ListStacks(ctx context.Context, req *defangv1.ListStacksRequest) (*defangv1.ListStacksResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &defangv1.ListStacksResponse{
		Stacks: m.stacks,
	}, nil
}

func (m *mockFabricClient) GetDefaultStack(ctx context.Context, req *defangv1.GetDefaultStackRequest) (*defangv1.GetStackResponse, error) {
	if m.defaultStack == nil {
		return nil, errors.New("no default stack set")
	}
	return &defangv1.GetStackResponse{
		Stack: m.defaultStack,
	}, nil
}

func TestNewManager(t *testing.T) {
	workingDir := "/tmp/test-dir"
	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, workingDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	assert.NotNil(t, manager, "NewManager should not return nil")
}

func TestManager_CreateListLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Test that listing returns empty when no stacks exist
	stacks, err := manager.List(t.Context())
	require.NoError(t, err, "List() should not error on empty directory")
	assert.Len(t, stacks, 0, "Expected empty stack list")

	// Test creating a stack
	params := Parameters{
		Name:     "teststack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
		Mode: modes.ModeAffordable,
	}

	filename, err := manager.Create(params)
	require.NoError(t, err, "Create() failed")

	expectedPath := filepath.Join(tmpDir, Directory, "teststack")
	assert.Equal(t, expectedPath, filename, "Expected filename mismatch")

	// Verify the file was created
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Error("Stack file was not created")
	}

	// Test listing after creating a stack
	stacks, err = manager.List(t.Context())

	require.NoError(t, err, "List() failed")
	assert.Len(t, stacks, 1, "Expected 1 stack")
	assert.Equal(t, "teststack", stacks[0].Name, "Expected stack name 'teststack'")
	assert.Equal(t, client.ProviderAWS, stacks[0].Provider, "Expected provider AWS")
	assert.Equal(t, "us-east-1", stacks[0].Region, "Expected region 'us-east-1'")
	assert.Equal(t, modes.ModeAffordable, stacks[0].Mode, "Expected mode 'AFFORDABLE'")

	// Test loading a stack
	loadedParams, err := manager.Load(t.Context(), "teststack")
	require.NoError(t, err, "Load() failed")
	assert.Equal(t, "teststack", loadedParams.Name, "Expected loaded stack name 'teststack'")
	assert.Equal(t, client.ProviderAWS, loadedParams.Provider, "Expected provider AWS")
	assert.Equal(t, "us-east-1", loadedParams.Region, "Expected region 'us-east-1'")
	assert.Equal(t, "default", loadedParams.Variables["AWS_PROFILE"], "Expected AWS profile 'default'")
	assert.Equal(t, modes.ModeAffordable, loadedParams.Mode, "Expected mode affordable")
}

func TestManager_CreateGCPStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Test creating a GCP stack
	params := Parameters{
		Name:     "gcpstack",
		Provider: client.ProviderGCP,
		Region:   "us-central1",
		Variables: map[string]string{
			"GCP_PROJECT_ID": "my-project",
		},
		Mode: modes.ModeBalanced,
	}

	filename, err := manager.Create(params)
	require.NoError(t, err, "Create() failed")

	expectedPath := filepath.Join(tmpDir, Directory, "gcpstack")
	assert.Equal(t, expectedPath, filename, "Expected filename mismatch")

	// Test loading the GCP stack
	loadedParams, err := manager.Load(t.Context(), "gcpstack")
	require.NoError(t, err, "Load() failed")
	assert.Equal(t, client.ProviderGCP, loadedParams.Provider, "Expected provider GCP")
	assert.Equal(t, "my-project", loadedParams.Variables["GCP_PROJECT_ID"], "Expected GCP project ID 'my-project'")
	assert.Equal(t, modes.ModeBalanced, loadedParams.Mode, "Expected mode balanced")
}

func TestManager_CreateMultipleStacks(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Create multiple stacks
	stacks := []Parameters{
		{
			Name:     "stack1",
			Provider: client.ProviderAWS,
			Region:   "us-east-1",
			Variables: map[string]string{
				"AWS_PROFILE": "default",
			},
			Mode: modes.ModeAffordable,
		},
		{
			Name:     "stack2",
			Provider: client.ProviderGCP,
			Region:   "us-west1",
			Variables: map[string]string{
				"GCP_PROJECT_ID": "project2",
			},
			Mode: modes.ModeHighAvailability,
		},
		{
			Name:     "stack3",
			Provider: client.ProviderDO,
			Region:   "nyc1",
			Mode:     modes.ModeBalanced,
		},
	}

	// Create all stacks
	for _, params := range stacks {
		_, err = manager.Create(params)
		require.NoError(t, err, "Create() failed for stack %s", params.Name)
	}

	// List all stacks
	listedStacks, err := manager.List(t.Context())
	require.NoError(t, err, "List() failed")

	assert.Len(t, listedStacks, 3, "Expected 3 stacks")

	// Verify each stack can be loaded
	for _, params := range stacks {
		loadedParams, err := manager.Load(t.Context(), params.Name)
		require.NoError(t, err, "Load() failed for stack %s", params.Name)
		assert.Equal(t, params.Name, loadedParams.Name, "Expected name %s", params.Name)
		assert.Equal(t, params.Provider, loadedParams.Provider, "Expected provider %s", params.Provider)
	}
}

func TestManager_LoadNonexistentStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Try to load a stack that doesn't exist
	_, err = manager.Load(t.Context(), "nonexistent")
	assert.Error(t, err, "Load() should return error for nonexistent stack")
}

func TestManager_CreateInvalidStackName(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Test with empty name
	params := Parameters{
		Name:     "",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
	}

	_, err = manager.Create(params)
	assert.Error(t, err, "Create() should return error for empty stack name")

	// Test with invalid characters
	params.Name = "Invalid-Stack-Name"
	_, err = manager.Create(params)
	assert.Error(t, err, "Create() should return error for invalid stack name with uppercase and hyphens")

	// Test with name starting with number
	params.Name = "1invalidname"
	_, err = manager.Create(params)
	assert.Error(t, err, "Create() should return error for stack name starting with number")
}

func TestManager_CreateDuplicateStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	params := Parameters{
		Name:     "duplicatestack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Mode:     modes.ModeAffordable,
	}

	// Create the first stack
	_, err = manager.Create(params)
	require.NoError(t, err, "First Create() failed")

	// Try to create the same stack again
	_, err = manager.Create(params)
	assert.Error(t, err, "Create() should return error for duplicate stack name")
}

func TestManager_ListRemote(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "remotestack1",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
			{
				Name: "remotestack2",
				StackFile: []byte(`
DEFANG_PROVIDER=gcp
GOOGLE_REGION=us-central1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
		},
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	remoteStacks, err := manager.ListRemote(t.Context())
	require.NoError(t, err, "ListRemote() failed")

	assert.Len(t, remoteStacks, 2, "Expected 2 remote stacks")

	assert.Equal(t, "remotestack1", remoteStacks[0].Name, "Expected stack name remotestack1")
	assert.Equal(t, client.ProviderAWS, remoteStacks[0].Provider, "Expected provider aws for remotestack1")
	assert.Equal(t, "us-east-1", remoteStacks[0].Region, "Expected region us-east-1 for remotestack1")
	assert.NotZero(t, remoteStacks[0].DeployedAt, "Expected DeployedAt to be set for remotestack1")

	assert.Equal(t, "remotestack2", remoteStacks[1].Name, "Expected stack name remotestack2")
	assert.Equal(t, client.ProviderGCP, remoteStacks[1].Provider, "Expected provider gcp for remotestack2")
	assert.Equal(t, "us-central1", remoteStacks[1].Region, "Expected region us-central1 for remotestack2")
	assert.NotZero(t, remoteStacks[1].DeployedAt, "Expected DeployedAt to be set for remotestack2")
}

func TestManager_ListRemoteError(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{
		listErr: errors.New("network error"),
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	_, err = manager.ListRemote(t.Context())
	assert.Error(t, err, "ListRemote() should return error when fabric client fails")
}

func TestManager_ListMerged(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "sharedstack",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
			{
				Name: "remoteonlystack",
				StackFile: []byte(`
DEFANG_PROVIDER=gcp
GOOGLE_REGION=us-central1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
		},
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager failed")

	// Create a local stack that exists remotely too
	localParams := Parameters{
		Name:     "sharedstack",
		Provider: client.ProviderAWS,
		Region:   "us-west-2", // Different region locally
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
		Mode: modes.ModeAffordable,
	}
	_, err = manager.Create(localParams)
	require.NoError(t, err, "First Create() failed")

	// Create a local-only stack
	localOnlyParams := Parameters{
		Name:     "localonlystack",
		Provider: client.ProviderAWS,
		Region:   "us-west-1",
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
		Mode: modes.ModeAffordable,
	}
	_, err = manager.Create(localOnlyParams)
	require.NoError(t, err, "Create() failed")

	// List merged stacks
	stacks, err := manager.List(t.Context())
	require.NoError(t, err, "List() failed")

	assert.Len(t, stacks, 3, "Expected 3 merged stacks")

	stackMap := make(map[string]ListItem)
	for _, stack := range stacks {
		stackMap[stack.Name] = stack
	}
	assert.Len(t, stackMap, 3, "Expected 3 unique stack names")

	// Check shared stack prefers remote (should have deployed time and local region)
	sharedStack, exists := stackMap["sharedstack"]
	if !exists {
		t.Error("Expected to find sharedstack")
	}
	assert.Equal(t, "us-west-2", sharedStack.Region, "Expected shared stack to use local region us-west-2")
	assert.Equal(t, client.ProviderAWS, sharedStack.Provider, "Expected shared stack to use provider aws")
	assert.Equal(t, modes.ModeAffordable, sharedStack.Mode, "Expected shared stack to use mode AFFORDABLE")
	assert.Equal(t, "default", sharedStack.Variables["AWS_PROFILE"], "Expected shared stack to have AWS_PROFILE variable from local stack")
	assert.Equal(t, deployedAt.Local().Format(time.RFC3339), sharedStack.DeployedAt.Local().Format(time.RFC3339), "Expected shared stack to have deployment time from remote")

	// Check remote-only stack exists
	_, exists = stackMap["remoteonlystack"]
	if !exists {
		t.Error("Expected to find remoteonlystack")
	}

	// Check local-only stack exists and has no deployed time
	localOnlyStack, exists := stackMap["localonlystack"]
	if !exists {
		t.Error("Expected to find localonlystack")
	}
	assert.Zero(t, localOnlyStack.DeployedAt, "Expected local-only stack to have zero deployed time")
}

func TestManager_ListRemoteWithBetaStack(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "", // Empty stack name should default to "beta"
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
		},
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager() failed")

	remoteStacks, err := manager.ListRemote(t.Context())
	require.NoError(t, err, "ListRemote() failed")

	assert.Len(t, remoteStacks, 1, "Expected 1 remote stack")
	assert.Equal(t, "beta", remoteStacks[0].Name, "Expected stack name to default to 'beta'")
}

func TestManager_ListRemoteDuplicateDeployments(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	olderDeployedAt := deployedAt.Add(-time.Hour)
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "duplicatestack",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
			{
				Name: "duplicatestack",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-west-2
`),
				LastDeployedAt: timestamppb.New(olderDeployedAt),
			},
		},
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager() failed")

	remoteStacks, err := manager.ListRemote(t.Context())
	require.NoError(t, err, "ListRemote() failed")

	assert.Len(t, remoteStacks, 2, "Expected 2 remote stacks")

	// Should be sorted by deployed time desc, so most recent (deployedAt) should be first
	assert.Equal(t, "duplicatestack", remoteStacks[0].Name, "Expected stack name 'duplicatestack'")
	assert.Equal(t, client.ProviderAWS, remoteStacks[0].Provider, "Expected provider from most recent deployment (aws)")
	assert.Equal(t, "us-east-1", remoteStacks[0].Region, "Expected region from most recent deployment (us-east-1)")
	assert.Equal(t, deployedAt.Local().Format(time.RFC3339), remoteStacks[0].DeployedAt.Local().Format(time.RFC3339), "Expected deployed time from most recent deployment")

	// Second stack should be the older one
	assert.Equal(t, "duplicatestack", remoteStacks[1].Name, "Expected stack name 'duplicatestack'")
	assert.Equal(t, client.ProviderAWS, remoteStacks[1].Provider, "Expected provider from older deployment (aws)")
	assert.Equal(t, "us-west-2", remoteStacks[1].Region, "Expected region from older deployment (us-west-2)")
	assert.Equal(t, olderDeployedAt.Local().Format(time.RFC3339), remoteStacks[1].DeployedAt.Local().Format(time.RFC3339), "Expected deployed time from older deployment")
}

func TestManager_WorkingDirectoryMatches(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager() failed")

	// Test that local operations work when working directory matches target directory
	params := Parameters{
		Name:     "teststack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
		Mode: modes.ModeAffordable,
	}

	// Create should work
	filename, err := manager.Create(params)
	require.NoError(t, err, "Create() failed when directories match")

	expectedPath := filepath.Join(tmpDir, Directory, "teststack")
	assert.Equal(t, expectedPath, filename, "Expected filename to match")

	// List should work
	stacks, err := manager.List(t.Context())
	require.NoError(t, err, "List() failed when directories match")

	assert.Len(t, stacks, 1, "Expected 1 stack when directories match")

	// Load should work
	loadedParams, err := manager.Load(t.Context(), "teststack")
	require.NoError(t, err, "Load() failed when directories match")

	assert.Equal(t, loadedParams.Name, "teststack", "Expected loaded stack name 'teststack'")
}

func TestManager_TargetDirectoryEmpty(t *testing.T) {
	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "remotestack1",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
			{
				Name: "remotestack2",
				StackFile: []byte(`
DEFANG_PROVIDER=gcp
GOOGLE_REGION=us-central1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
		},
	}
	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, "", "test-project", ec)
	require.NoError(t, err, "NewManager() failed")

	// Test that local operations are blocked when working directory differs from target directory
	params := Parameters{
		Name:     "teststack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Variables: map[string]string{
			"AWS_PROFILE": "default",
		},
		Mode: modes.ModeAffordable,
	}

	// Create should fail
	_, err = manager.Create(params)
	require.Error(t, err, "Create() should fail when target directory is empty")
	require.Contains(t, err.Error(), "Create not allowed: target directory", "Expected specific error message about operation not allowed")

	// List should return only remote stacks (no error)
	stacks, err := manager.List(t.Context())
	require.NoError(t, err, "List() should not fail when target directory is empty")
	assert.Len(t, stacks, 2, "Expected 2 remote stacks when target directory is empty")

	// Verify the returned stacks are remote stacks
	stackNames := make(map[string]bool)
	for _, stack := range stacks {
		stackNames[stack.Name] = true
		assert.NotZero(t, stack.DeployedAt, "Expected DeployedAt to be set for remote stacks")
	}
	require.True(t, stackNames["remotestack1"], "Expected to find remotestack1")
	require.True(t, stackNames["remotestack2"], "Expected to find remotestack2")

	// Load should fail
	_, err = manager.Load(t.Context(), "teststack")
	require.Error(t, err, "Load() should fail when target directory is empty")
	require.Contains(t, err.Error(), "unable to find stack \"teststack\"", "Expected specific error message about operation not allowed")
}

func TestManager_RemoteOperationsWorkRegardlessOfDirectory(t *testing.T) {
	// Create a temporary directory for testing but don't change to it
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		stacks: []*defangv1.Stack{
			{
				Name: "remotestack",
				StackFile: []byte(`
DEFANG_PROVIDER=aws
AWS_REGION=us-east-1
`),
				LastDeployedAt: timestamppb.New(deployedAt),
			},
		},
	}

	ec := &mockElicitationsController{supported: true}
	manager, err := NewManager(mockClient, tmpDir, "test-project", ec)
	require.NoError(t, err, "NewManager() failed")

	// Remote operations should work even when directories don't match
	remoteStacks, err := manager.ListRemote(t.Context())
	require.NoError(t, err, "ListRemote() should work even when directories don't match")

	assert.Len(t, remoteStacks, 1, "Expected 1 remote stack when directories don't match")
	assert.Equal(t, remoteStacks[0].Name, "remotestack", "Expected stack name 'remotestack'")
}

func TestGetStack(t *testing.T) {
	tests := []struct {
		name                 string
		projectName          string
		options              GetStackOpts
		defaultStack         *defangv1.Stack
		localStack           *Parameters
		remoteStack          *Parameters
		expectedError        string
		expectedStack        *Parameters
		expectedEnv          map[string]string
		interactiveResponses map[string]string
	}{
		{
			name:        "stack specified but not found",
			projectName: "foo",
			options: GetStackOpts{
				Stack: "missingstack",
			},
			expectedError: "unable to find stack",
			expectedEnv:   map[string]string{},
		},
		{
			name:        "local stack specified",
			projectName: "foo",
			options: GetStackOpts{
				Stack: "localstack",
			},
			localStack: &Parameters{
				Name:     "localstack",
				Provider: client.ProviderDefang,
				Region:   "us-test-2",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "defang",
					"FOO":             "bar",
				},
			},
			expectedStack: &Parameters{
				Name:      "localstack",
				Provider:  client.ProviderDefang,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "defang",
				"FOO":             "bar",
			},
		},
		{
			name:        "remote stack specified",
			projectName: "foo",
			options: GetStackOpts{
				Stack: "remotestack",
			},
			remoteStack: &Parameters{
				Name:     "remotestack",
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "gcp",
					"GCP_PROJECT_ID":  "my-gcp-project",
					"FOO":             "bar",
				},
			},
			expectedStack: &Parameters{
				Name:      "remotestack",
				Provider:  client.ProviderGCP,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "gcp",
				"GCP_PROJECT_ID":  "my-gcp-project",
				"FOO":             "bar",
			},
		},
		{
			name:        "local and remote stack",
			projectName: "foo",
			options: GetStackOpts{
				Stack: "bothstack",
			},
			localStack: &Parameters{
				Name:     "bothstack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "aws",
					"AWS_PROFILE":     "local-profile",
					"FOO":             "local-bar",
				},
			},
			remoteStack: &Parameters{
				Name:     "bothstack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "aws",
					"AWS_PROFILE":     "remote-profile",
					"FOO":             "remote-bar",
				},
			},
			expectedStack: &Parameters{
				Name:     "bothstack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "aws",
					"AWS_PROFILE":     "local-profile",
					"FOO":             "local-bar",
				},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "aws",
				"AWS_PROFILE":     "local-profile",
				"FOO":             "local-bar",
			},
		},
		{
			name:        "interactive selection - stack required",
			projectName: "foo",
			options: GetStackOpts{
				Interactive:        true,
				AllowStackCreation: true,
				RequireStack:       true,
			},
			remoteStack: &Parameters{
				Name:     "existingstack",
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "gcp",
					"GCP_PROJECT":     "existing-gcp-project",
					"FOO":             "existing-bar",
				},
			},
			interactiveResponses: map[string]string{
				"stack": "existingstack",
			},
			expectedStack: &Parameters{
				Name:      "existingstack",
				Provider:  client.ProviderGCP,
				Variables: map[string]string{},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "gcp",
				"GCP_PROJECT":     "existing-gcp-project",
				"FOO":             "existing-bar",
			},
		},
		{
			name:        "interactive selection - stack not required, fallback to default",
			projectName: "foo",
			options: GetStackOpts{
				Interactive:        true,
				AllowStackCreation: true,
			},
			defaultStack: &defangv1.Stack{
				Name:     "mydefault",
				Provider: defangv1.Provider_GCP,
				StackFile: []byte(`
DEFANG_PROVIDER=gcp
`),
			},
			remoteStack: &Parameters{
				Name:     "existingstack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Variables: map[string]string{
					"DEFANG_PROVIDER": "aws",
					"FOO":             "existing-bar",
				},
			},
			expectedStack: &Parameters{
				Name:     "mydefault",
				Provider: client.ProviderGCP,
			},
		},
		{
			name:        "stack with compose vars updates loader",
			projectName: "foo",
			options: GetStackOpts{
				Stack: "composestack",
			},
			localStack: &Parameters{
				Name:     "composestack",
				Provider: client.ProviderDefang,
				Region:   "us-test-2",
				Variables: map[string]string{
					"COMPOSE_PROJECT_NAME": "myproject",
					"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
				},
			},
			expectedStack: &Parameters{
				Name:     "composestack",
				Provider: client.ProviderDefang,
				Variables: map[string]string{
					"COMPOSE_PROJECT_NAME": "myproject",
					"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
				},
			},
			expectedEnv: map[string]string{
				"COMPOSE_PROJECT_NAME": "myproject",
				"COMPOSE_PATH":         "./docker-compose.yml:./docker-compose.override.yml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary directory for testing
			tmpDir := t.TempDir()

			// Change to temp directory so working directory matches target directory
			t.Chdir(tmpDir)
			ctx := t.Context()

			ec := &mockElicitationsController{
				supported:     true,
				enumResponses: tt.interactiveResponses,
			}

			if tt.localStack != nil {
				_, err := CreateInDirectory(tmpDir, *tt.localStack)
				require.NoError(t, err, "Failed to create local stack")
			}

			remoteStacks := []*defangv1.Stack{}
			if tt.remoteStack != nil {
				var stackFileBuilder strings.Builder
				for key, value := range tt.remoteStack.Variables {
					stackFileBuilder.WriteString(key)
					stackFileBuilder.WriteString("=")
					stackFileBuilder.WriteString(value)
					stackFileBuilder.WriteString("\n")
				}
				stackFileContent := stackFileBuilder.String()
				remoteStacks = append(remoteStacks, &defangv1.Stack{
					Name:      tt.remoteStack.Name,
					StackFile: []byte(stackFileContent),
				})
			}

			var mockFabric *mockFabricClient
			if tt.defaultStack == nil {
				mockFabric = &mockFabricClient{
					stacks: remoteStacks,
				}
			} else {
				mockFabric = &mockFabricClient{
					stacks:       remoteStacks,
					defaultStack: tt.defaultStack,
				}
			}

			targetDirectory := "."
			manager, err := NewManager(mockFabric, targetDirectory, tt.projectName, ec)
			require.NoError(t, err, "Failed to create Manager")
			stack, _, err := manager.GetStack(ctx, tt.options)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, stack)
			assert.Equal(t, tt.expectedStack.Name, stack.Name)
			assert.Equal(t, tt.expectedStack.Provider, stack.Provider)

			// Verify environment variables
			for key, expectedValue := range tt.expectedEnv {
				actualValue, exists := stack.Variables[key]
				assert.True(t, exists, "expected env var %s to be set", key)
				assert.Equal(t, expectedValue, actualValue, "env var %s has unexpected value", key)
			}
		})
	}
}
