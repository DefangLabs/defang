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
	"google.golang.org/protobuf/types/known/timestamppb"
)

// mockFabricClient implements FabricClient interface for testing
type mockFabricClient struct {
	deployments []*defangv1.Deployment
	listErr     error
}

func (m *mockFabricClient) ListDeployments(ctx context.Context, req *defangv1.ListDeploymentsRequest) (*defangv1.ListDeploymentsResponse, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return &defangv1.ListDeploymentsResponse{
		Deployments: m.deployments,
	}, nil
}

func TestNewManager(t *testing.T) {
	workingDir := "/tmp/test-dir"
	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, workingDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager == nil {
		t.Error("NewManager should not return nil")
	}
}

func TestManager_CreateListLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test that listing returns empty when no stacks exist
	stacks, err := manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() should not error on empty directory: %v", err)
	}
	if len(stacks) != 0 {
		t.Errorf("Expected empty stack list, got %d stacks", len(stacks))
	}

	// Test creating a stack
	params := StackParameters{
		Name:       "teststack",
		Provider:   client.ProviderAWS,
		Region:     "us-east-1",
		AWSProfile: "default",
		Mode:       modes.ModeAffordable,
	}

	filename, err := manager.Create(params)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, Directory, "teststack")
	if filename != expectedPath {
		t.Errorf("Expected filename %s, got %s", expectedPath, filename)
	}

	// Verify the file was created
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Error("Stack file was not created")
	}

	// Test listing after creating a stack
	stacks, err = manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(stacks) != 1 {
		t.Errorf("Expected 1 stack, got %d", len(stacks))
	}
	if stacks[0].Name != "teststack" {
		t.Errorf("Expected stack name 'teststack', got '%s'", stacks[0].Name)
	}
	if stacks[0].Provider != "aws" {
		t.Errorf("Expected provider 'aws', got '%s'", stacks[0].Provider)
	}
	if stacks[0].Region != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got '%s'", stacks[0].Region)
	}
	if stacks[0].Mode != "AFFORDABLE" {
		t.Errorf("Expected mode 'AFFORDABLE', got '%s'", stacks[0].Mode)
	}

	// Test loading a stack
	loadedParams, err := manager.Load(t.Context(), "teststack")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if loadedParams.Name != "teststack" {
		t.Errorf("Expected loaded stack name 'teststack', got '%s'", loadedParams.Name)
	}
	if loadedParams.Provider != client.ProviderAWS {
		t.Errorf("Expected provider AWS, got %s", loadedParams.Provider)
	}
	if loadedParams.Region != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got '%s'", loadedParams.Region)
	}
	if loadedParams.AWSProfile != "default" {
		t.Errorf("Expected AWS profile 'default', got '%s'", loadedParams.AWSProfile)
	}
	if loadedParams.Mode != modes.ModeAffordable {
		t.Errorf("Expected mode affordable, got %s", loadedParams.Mode)
	}
}

func TestManager_CreateGCPStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test creating a GCP stack
	params := StackParameters{
		Name:         "gcpstack",
		Provider:     client.ProviderGCP,
		Region:       "us-central1",
		GCPProjectID: "my-project",
		Mode:         modes.ModeBalanced,
	}

	filename, err := manager.Create(params)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, Directory, "gcpstack")
	if filename != expectedPath {
		t.Errorf("Expected filename %s, got %s", expectedPath, filename)
	}

	// Test loading the GCP stack
	loadedParams, err := manager.Load(t.Context(), "gcpstack")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}
	if loadedParams.Provider != client.ProviderGCP {
		t.Errorf("Expected provider GCP, got %s", loadedParams.Provider)
	}
	if loadedParams.GCPProjectID != "my-project" {
		t.Errorf("Expected GCP project ID 'my-project', got '%s'", loadedParams.GCPProjectID)
	}
	if loadedParams.Mode != modes.ModeBalanced {
		t.Errorf("Expected mode balanced, got %s", loadedParams.Mode)
	}
}

func TestManager_CreateMultipleStacks(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create multiple stacks
	stacks := []StackParameters{
		{
			Name:       "stack1",
			Provider:   client.ProviderAWS,
			Region:     "us-east-1",
			AWSProfile: "profile1",
			Mode:       modes.ModeAffordable,
		},
		{
			Name:         "stack2",
			Provider:     client.ProviderGCP,
			Region:       "us-west1",
			GCPProjectID: "project2",
			Mode:         modes.ModeHighAvailability,
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
		if err != nil {
			t.Fatalf("Create() failed for stack %s: %v", params.Name, err)
		}
	}

	// List all stacks
	listedStacks, err := manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(listedStacks) != 3 {
		t.Errorf("Expected 3 stacks, got %d", len(listedStacks))
	}

	// Verify each stack can be loaded
	for _, params := range stacks {
		loadedParams, err := manager.Load(t.Context(), params.Name)
		if err != nil {
			t.Fatalf("Load() failed for stack %s: %v", params.Name, err)
		}
		if loadedParams.Name != params.Name {
			t.Errorf("Expected name %s, got %s", params.Name, loadedParams.Name)
		}
		if loadedParams.Provider != params.Provider {
			t.Errorf("Expected provider %s, got %s", params.Provider, loadedParams.Provider)
		}
	}
}

func TestManager_LoadNonexistentStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Try to load a stack that doesn't exist
	_, err = manager.Load(t.Context(), "nonexistent")
	if err == nil {
		t.Error("Load() should return error for nonexistent stack")
	}
}

func TestManager_CreateInvalidStackName(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test with empty name
	params := StackParameters{
		Name:     "",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
	}

	_, err = manager.Create(params)
	if err == nil {
		t.Error("Create() should return error for empty stack name")
	}

	// Test with invalid characters
	params.Name = "Invalid-Stack-Name"
	_, err = manager.Create(params)
	if err == nil {
		t.Error("Create() should return error for invalid stack name with uppercase and hyphens")
	}

	// Test with name starting with number
	params.Name = "1invalidname"
	_, err = manager.Create(params)
	if err == nil {
		t.Error("Create() should return error for stack name starting with number")
	}
}

func TestManager_CreateDuplicateStack(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	params := StackParameters{
		Name:     "duplicatestack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Mode:     modes.ModeAffordable,
	}

	// Create the first stack
	_, err = manager.Create(params)
	if err != nil {
		t.Fatalf("First Create() failed: %v", err)
	}

	// Try to create the same stack again
	_, err = manager.Create(params)
	if err == nil {
		t.Error("Create() should return error for duplicate stack name")
	}
}

func TestManager_ListRemote(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "remotestack1",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt),
			},
			{
				Stack:     "remotestack2",
				Provider:  defangv1.Provider_GCP,
				Region:    "us-central1",
				Timestamp: timestamppb.New(deployedAt.Add(-time.Hour)),
			},
		},
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	remoteStacks, err := manager.ListRemote(t.Context())
	if err != nil {
		t.Fatalf("ListRemote() failed: %v", err)
	}

	if len(remoteStacks) != 2 {
		t.Errorf("Expected 2 remote stacks, got %d", len(remoteStacks))
	}

	// Check first remote stack
	if remoteStacks[0].Name != "remotestack1" && remoteStacks[1].Name != "remotestack1" {
		t.Error("Expected to find remotestack1")
	}

	// Check second remote stack
	if remoteStacks[0].Name != "remotestack2" && remoteStacks[1].Name != "remotestack2" {
		t.Error("Expected to find remotestack2")
	}

	// Verify deployed time is set
	for _, stack := range remoteStacks {
		if stack.DeployedAt.IsZero() {
			t.Errorf("Expected DeployedAt to be set for stack %s", stack.Name)
		}
	}
}

func TestManager_ListRemoteError(t *testing.T) {
	tmpDir := t.TempDir()

	mockClient := &mockFabricClient{
		listErr: errors.New("network error"),
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, err = manager.ListRemote(t.Context())
	if err == nil {
		t.Error("ListRemote() should return error when fabric client fails")
	}
}

func TestManager_ListMerged(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "sharedstack",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt),
			},
			{
				Stack:     "remoteonlystack",
				Provider:  defangv1.Provider_GCP,
				Region:    "us-central1",
				Timestamp: timestamppb.New(deployedAt),
			},
		},
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Create a local stack that exists remotely too
	localParams := StackParameters{
		Name:       "sharedstack",
		Provider:   client.ProviderAWS,
		Region:     "us-west-2", // Different region locally
		AWSProfile: "default",
		Mode:       modes.ModeAffordable,
	}
	_, err = manager.Create(localParams)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// Create a local-only stack
	localOnlyParams := StackParameters{
		Name:       "localonlystack",
		Provider:   client.ProviderAWS,
		Region:     "us-west-1",
		AWSProfile: "default",
		Mode:       modes.ModeAffordable,
	}
	_, err = manager.Create(localOnlyParams)
	if err != nil {
		t.Fatalf("Create() failed: %v", err)
	}

	// List merged stacks
	stacks, err := manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(stacks) != 3 {
		t.Errorf("Expected 3 merged stacks, got %d", len(stacks))
	}

	stackMap := make(map[string]StackListItem)
	for _, stack := range stacks {
		stackMap[stack.Name] = stack
	}

	// Check shared stack prefers remote (should have deployed time and remote region)
	sharedStack, exists := stackMap["sharedstack"]
	if !exists {
		t.Error("Expected to find sharedstack")
	} else {
		if sharedStack.Region != "us-east-1" {
			t.Errorf("Expected shared stack to use remote region us-east-1, got %s", sharedStack.Region)
		}
		if sharedStack.DeployedAt.IsZero() {
			t.Error("Expected shared stack to have deployment time from remote")
		}
	}

	// Check remote-only stack exists
	_, exists = stackMap["remoteonlystack"]
	if !exists {
		t.Error("Expected to find remoteonlystack")
	}

	// Check local-only stack exists and has no deployed time
	localOnlyStack, exists := stackMap["localonlystack"]
	if !exists {
		t.Error("Expected to find localonlystack")
	} else {
		if !localOnlyStack.DeployedAt.IsZero() {
			t.Error("Expected local-only stack to have zero deployed time")
		}
	}
}

func TestManager_ListRemoteWithBetaStack(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "", // Empty stack name should default to "beta"
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt),
			},
		},
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	remoteStacks, err := manager.ListRemote(t.Context())
	if err != nil {
		t.Fatalf("ListRemote() failed: %v", err)
	}

	if len(remoteStacks) != 1 {
		t.Errorf("Expected 1 remote stack, got %d", len(remoteStacks))
	}

	if remoteStacks[0].Name != "beta" {
		t.Errorf("Expected stack name to be 'beta', got '%s'", remoteStacks[0].Name)
	}
}

func TestManager_ListRemoteDuplicateDeployments(t *testing.T) {
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "duplicatestack",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt), // Most recent
			},
			{
				Stack:     "duplicatestack",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-west-2",
				Timestamp: timestamppb.New(deployedAt.Add(-time.Hour)), // Older
			},
		},
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	remoteStacks, err := manager.ListRemote(t.Context())
	if err != nil {
		t.Fatalf("ListRemote() failed: %v", err)
	}

	if len(remoteStacks) != 1 {
		t.Errorf("Expected 1 remote stack (duplicates should be merged), got %d", len(remoteStacks))
	}

	// Should use the first deployment (most recent) since they're already sorted
	if remoteStacks[0].Region != "us-east-1" {
		t.Errorf("Expected region from first deployment (us-east-1), got %s", remoteStacks[0].Region)
	}
}

func TestManager_WorkingDirectoryMatches(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory so working directory matches target directory
	t.Chdir(tmpDir)

	mockClient := &mockFabricClient{}
	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test that local operations work when working directory matches target directory
	params := StackParameters{
		Name:       "teststack",
		Provider:   client.ProviderAWS,
		Region:     "us-east-1",
		AWSProfile: "default",
		Mode:       modes.ModeAffordable,
	}

	// Create should work
	filename, err := manager.Create(params)
	if err != nil {
		t.Fatalf("Create() failed when directories match: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, Directory, "teststack")
	if filename != expectedPath {
		t.Errorf("Expected filename %s, got %s", expectedPath, filename)
	}

	// List should work
	stacks, err := manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() failed when directories match: %v", err)
	}

	if len(stacks) != 1 {
		t.Errorf("Expected 1 stack, got %d", len(stacks))
	}

	// Load should work
	loadedParams, err := manager.Load(t.Context(), "teststack")
	if err != nil {
		t.Fatalf("Load() failed when directories match: %v", err)
	}

	if loadedParams.Name != "teststack" {
		t.Errorf("Expected loaded stack name 'teststack', got '%s'", loadedParams.Name)
	}
}

func TestManager_TargetDirectoryEmpty(t *testing.T) {
	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "remotestack1",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt),
			},
			{
				Stack:     "remotestack2",
				Provider:  defangv1.Provider_GCP,
				Region:    "us-central1",
				Timestamp: timestamppb.New(deployedAt),
			},
		},
	}
	manager, err := NewManager(mockClient, "", "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test that local operations are blocked when working directory differs from target directory
	params := StackParameters{
		Name:       "teststack",
		Provider:   client.ProviderAWS,
		Region:     "us-east-1",
		AWSProfile: "default",
		Mode:       modes.ModeAffordable,
	}

	// Create should fail
	_, err = manager.Create(params)
	if err == nil {
		t.Fatal("Create() should fail when target directory is empty")
	}
	if !strings.Contains(err.Error(), "Create not allowed: target directory") {
		t.Errorf("Expected specific error message about operation not allowed, got: %v", err)
	}

	// List should return only remote stacks (no error)
	stacks, err := manager.List(t.Context())
	if err != nil {
		t.Fatalf("List() should not fail when target directory is empty: %v", err)
	}
	if len(stacks) != 2 {
		t.Errorf("Expected 2 remote stacks, got %d", len(stacks))
	}

	// Verify the returned stacks are remote stacks
	stackNames := make(map[string]bool)
	for _, stack := range stacks {
		stackNames[stack.Name] = true
		if stack.DeployedAt.IsZero() {
			t.Errorf("Expected remote stack %s to have deployment time", stack.Name)
		}
	}
	if !stackNames["remotestack1"] {
		t.Error("Expected to find remotestack1")
	}
	if !stackNames["remotestack2"] {
		t.Error("Expected to find remotestack2")
	}

	// Load should fail
	_, err = manager.Load(t.Context(), "teststack")
	if err == nil {
		t.Fatal("Load() should fail when target directory is empty")
	}
	if !strings.Contains(err.Error(), "unable to find stack \"teststack\"") {
		t.Errorf("Expected specific error message about operation not allowed, got: %v", err)
	}
}

func TestManager_RemoteOperationsWorkRegardlessOfDirectory(t *testing.T) {
	// Create a temporary directory for testing but don't change to it
	tmpDir := t.TempDir()

	deployedAt := time.Now()
	mockClient := &mockFabricClient{
		deployments: []*defangv1.Deployment{
			{
				Stack:     "remotestack",
				Provider:  defangv1.Provider_AWS,
				Region:    "us-east-1",
				Timestamp: timestamppb.New(deployedAt),
			},
		},
	}

	manager, err := NewManager(mockClient, tmpDir, "test-project")
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Remote operations should work even when directories don't match
	remoteStacks, err := manager.ListRemote(t.Context())
	if err != nil {
		t.Fatalf("ListRemote() should work even when directories don't match: %v", err)
	}

	if len(remoteStacks) != 1 {
		t.Errorf("Expected 1 remote stack, got %d", len(remoteStacks))
	}

	if remoteStacks[0].Name != "remotestack" {
		t.Errorf("Expected stack name 'remotestack', got '%s'", remoteStacks[0].Name)
	}
}
