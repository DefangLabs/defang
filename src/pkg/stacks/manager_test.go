package stacks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
)

func TestNewManager(t *testing.T) {
	workingDir := "/tmp/test-dir"
	manager := NewManager(workingDir)

	if manager == nil {
		t.Error("NewManager should not return nil")
	}
}

func TestManager_CreateListLoad(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	manager := NewManager(tmpDir)

	// Test that listing returns empty when no stacks exist
	stacks, err := manager.List()
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
	stacks, err = manager.List()
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
	loadedParams, err := manager.Load("teststack")
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

	manager := NewManager(tmpDir)

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
	loadedParams, err := manager.Load("gcpstack")
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

	manager := NewManager(tmpDir)

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
		_, err := manager.Create(params)
		if err != nil {
			t.Fatalf("Create() failed for stack %s: %v", params.Name, err)
		}
	}

	// List all stacks
	listedStacks, err := manager.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	if len(listedStacks) != 3 {
		t.Errorf("Expected 3 stacks, got %d", len(listedStacks))
	}

	// Verify each stack can be loaded
	for _, params := range stacks {
		loadedParams, err := manager.Load(params.Name)
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

	manager := NewManager(tmpDir)

	// Try to load a stack that doesn't exist
	_, err := manager.Load("nonexistent")
	if err == nil {
		t.Error("Load() should return error for nonexistent stack")
	}
}

func TestManager_CreateInvalidStackName(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	manager := NewManager(tmpDir)

	// Test with empty name
	params := StackParameters{
		Name:     "",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
	}

	_, err := manager.Create(params)
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

	manager := NewManager(tmpDir)

	params := StackParameters{
		Name:     "duplicatestack",
		Provider: client.ProviderAWS,
		Region:   "us-east-1",
		Mode:     modes.ModeAffordable,
	}

	// Create the first stack
	_, err := manager.Create(params)
	if err != nil {
		t.Fatalf("First Create() failed: %v", err)
	}

	// Try to create the same stack again
	_, err = manager.Create(params)
	if err == nil {
		t.Error("Create() should return error for duplicate stack name")
	}
}
