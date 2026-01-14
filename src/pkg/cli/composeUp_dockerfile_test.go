package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
)

func TestComposeUp_DockerfileValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		projectPath   string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid Dockerfile in build testdata",
			projectPath: "../../testdata/build",
			expectError: false,
		},
		{
			name:          "Invalid Dockerfile (missing FROM)",
			projectPath:   "../../testdata/dockerfile-validation-errors",
			expectError:   true,
			errorContains: "must contain at least one FROM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Load the project
			loader := compose.NewLoader(compose.WithPath(tt.projectPath + "/compose.yaml"))
			project, err := loader.LoadProject(ctx)
			if err != nil {
				t.Fatalf("Failed to load project: %v", err)
			}

			stack := &stacks.StackParameters{
				Provider: client.ProviderDefang, // Use a valid provider to avoid panic
			}

			// Try to do a ComposeUp with the project
			mockFabric := client.MockFabricClient{DelegateDomain: "example.com"}
			mockProvider := &mockDeployProvider{MockProvider: client.MockProvider{}}

			_, _, err = ComposeUp(ctx, mockFabric, mockProvider, stack, ComposeUpParams{
				Project:    project,
				UploadMode: compose.UploadModeDigest, // This should trigger validation
				Mode:       modes.ModeUnspecified,
			})

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil && !strings.Contains(err.Error(), "failed to get delegate domain") {
					// It's okay to fail on delegation or other issues since we're using mocks
					// but not on Dockerfile validation
					if strings.Contains(err.Error(), "Dockerfile") {
						t.Errorf("Expected no Dockerfile validation error, got: %v", err)
					}
				}
			}
		})
	}
}

func TestComposeUp_DockerfileValidationSkipped(t *testing.T) {
	ctx := context.Background()

	// Load project with invalid Dockerfile
	loader := compose.NewLoader(compose.WithPath("../../testdata/dockerfile-validation-errors/compose.yaml"))
	project, err := loader.LoadProject(ctx)
	if err != nil {
		t.Fatalf("Failed to load project: %v", err)
	}

	stack := &stacks.StackParameters{
		Provider: client.ProviderDefang, // Use a valid provider to avoid panic
	}

	mockFabric := client.MockFabricClient{DelegateDomain: "example.com"}
	mockProvider := &mockDeployProvider{MockProvider: client.MockProvider{}}

	// Test that validation is skipped for UploadModeIgnore (dry-run)
	_, _, err = ComposeUp(ctx, mockFabric, mockProvider, stack, ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeIgnore, // Should skip validation
		Mode:       modes.ModeUnspecified,
	})

	// Should not get Dockerfile validation error
	if err != nil && strings.Contains(err.Error(), "Dockerfile") && strings.Contains(err.Error(), "FROM") {
		t.Errorf("Validation should be skipped for UploadModeIgnore, got: %v", err)
	}

	// Test that validation is skipped for UploadModeEstimate
	_, _, err = ComposeUp(ctx, mockFabric, mockProvider, stack, ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModeEstimate, // Should skip validation
		Mode:       modes.ModeUnspecified,
	})

	// Should not get Dockerfile validation error
	if err != nil && strings.Contains(err.Error(), "Dockerfile") && strings.Contains(err.Error(), "FROM") {
		t.Errorf("Validation should be skipped for UploadModeEstimate, got: %v", err)
	}
}
