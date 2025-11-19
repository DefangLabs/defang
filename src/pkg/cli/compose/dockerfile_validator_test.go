package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDockerfile(t *testing.T) {
	// Create a temporary directory for test Dockerfiles
	tmpDir := t.TempDir()

	tests := []struct {
		name          string
		dockerfile    string
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid Dockerfile",
			dockerfile: `FROM alpine:latest
RUN echo "hello"
CMD ["echo", "world"]`,
			expectError: false,
		},
		{
			name: "Valid Dockerfile with ARG before FROM",
			dockerfile: `ARG BASE_IMAGE=alpine
FROM ${BASE_IMAGE}
RUN echo "hello"`,
			expectError: false,
		},
		{
			name: "Valid Dockerfile with multiple stages",
			dockerfile: `FROM alpine AS builder
RUN echo "build"
FROM alpine
COPY --from=builder /app /app`,
			expectError: false,
		},
		{
			name:          "Empty Dockerfile",
			dockerfile:    "",
			expectError:   true,
			errorContains: "empty or contains only comments",
		},
		{
			name: "Dockerfile with only comments",
			dockerfile: `# This is a comment
# Another comment`,
			expectError:   true,
			errorContains: "empty or contains only comments",
		},
		{
			name: "Missing FROM instruction",
			dockerfile: `RUN echo "hello"
CMD ["echo", "world"]`,
			expectError:   true,
			errorContains: "must contain at least one FROM",
		},
		{
			name: "RUN before FROM (invalid)",
			dockerfile: `RUN echo "hello"
FROM alpine`,
			expectError:   true,
			errorContains: "must come after FROM",
		},
		{
			name: "COPY before FROM (invalid)",
			dockerfile: `COPY . /app
FROM alpine`,
			expectError:   true,
			errorContains: "must come after FROM",
		},
		{
			name: "Valid Dockerfile with trailing whitespace",
			dockerfile: `FROM alpine:latest
RUN echo "hello"
CMD ["echo", "world"]  `,
			expectError: false,
		},
		{
			name: "Valid Dockerfile with MAINTAINER (deprecated but valid)",
			dockerfile: `FROM alpine:latest
MAINTAINER test@example.com
RUN echo "hello"`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test Dockerfile
			dockerfilePath := filepath.Join(tmpDir, "Dockerfile."+strings.ReplaceAll(tt.name, " ", "_"))
			err := os.WriteFile(dockerfilePath, []byte(tt.dockerfile), 0644)
			if err != nil {
				t.Fatalf("Failed to create test Dockerfile: %v", err)
			}

			// Validate the Dockerfile
			err = ValidateDockerfile(dockerfilePath, "test-service")

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestValidateDockerfileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.dockerfile")

	err := ValidateDockerfile(nonExistentPath, "test-service")
	if err == nil {
		t.Error("Expected error for non-existent Dockerfile")
	}
	if !strings.Contains(err.Error(), "failed to read Dockerfile") {
		t.Errorf("Expected 'failed to read Dockerfile' error, got: %v", err)
	}
}

func TestValidateServiceDockerfiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test Dockerfiles
	validDockerfile := filepath.Join(tmpDir, "Dockerfile.valid")
	err := os.WriteFile(validDockerfile, []byte("FROM alpine\nRUN echo hello"), 0644)
	if err != nil {
		t.Fatalf("Failed to create valid Dockerfile: %v", err)
	}

	invalidDockerfile := filepath.Join(tmpDir, "Dockerfile.invalid")
	err = os.WriteFile(invalidDockerfile, []byte("RUN echo hello"), 0644)
	if err != nil {
		t.Fatalf("Failed to create invalid Dockerfile: %v", err)
	}

	tests := []struct {
		name          string
		project       *Project
		expectError   bool
		errorContains string
	}{
		{
			name: "No services with build",
			project: &Project{
				Services: Services{
					"web": {
						Name:  "web",
						Image: "nginx",
					},
				},
			},
			expectError: false,
		},
		{
			name: "Valid Dockerfile",
			project: &Project{
				Services: Services{
					"app": {
						Name: "app",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: "Dockerfile.valid",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid Dockerfile",
			project: &Project{
				Services: Services{
					"app": {
						Name: "app",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: "Dockerfile.invalid",
						},
					},
				},
			},
			expectError:   true,
			errorContains: "must contain at least one FROM",
		},
		{
			name: "Multiple services with mixed validity",
			project: &Project{
				Services: Services{
					"valid": {
						Name: "valid",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: "Dockerfile.valid",
						},
					},
					"invalid": {
						Name: "invalid",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: "Dockerfile.invalid",
						},
					},
				},
			},
			expectError:   true,
			errorContains: "must contain at least one FROM",
		},
		{
			name: "Service with Railpack marker",
			project: &Project{
				Services: Services{
					"app": {
						Name: "app",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: RAILPACK,
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "Service with non-existent Dockerfile (skipped)",
			project: &Project{
				Services: Services{
					"app": {
						Name: "app",
						Build: &BuildConfig{
							Context:    tmpDir,
							Dockerfile: "does-not-exist",
						},
					},
				},
			},
			expectError: false, // Non-existent files are skipped
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateServiceDockerfiles(tt.project)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestDockerfileValidationError(t *testing.T) {
	tests := []struct {
		name     string
		err      *DockerfileValidationError
		expected string
	}{
		{
			name: "Error without line number",
			err: &DockerfileValidationError{
				ServiceName:    "web",
				DockerfilePath: "/path/to/Dockerfile",
				Message:        "missing FROM instruction",
			},
			expected: `service "web": "/path/to/Dockerfile":
	missing FROM instruction`,
		},
		{
			name: "Error with line number",
			err: &DockerfileValidationError{
				ServiceName:    "api",
				DockerfilePath: "/path/to/Dockerfile",
				Line:           5,
				Message:        "invalid syntax",
			},
			expected: `service "api": "/path/to/Dockerfile" at line 5:
	invalid syntax`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Error message mismatch:\ngot:  %s\nwant: %s", got, tt.expected)
			}
		})
	}
}

func TestDockerfileValidationErrors(t *testing.T) {
	errors := &DockerfileValidationErrors{
		Errors: []error{
			&DockerfileValidationError{
				ServiceName:    "web",
				DockerfilePath: "/path/to/Dockerfile1",
				Message:        "error 1",
			},
			&DockerfileValidationError{
				ServiceName:    "api",
				DockerfilePath: "/path/to/Dockerfile2",
				Message:        "error 2",
			},
		},
	}

	errMsg := errors.Error()
	if !strings.Contains(errMsg, "Dockerfile validation failed") {
		t.Error("Error message should contain 'Dockerfile validation failed'")
	}
	if !strings.Contains(errMsg, "error 1") {
		t.Error("Error message should contain 'error 1'")
	}
	if !strings.Contains(errMsg, "error 2") {
		t.Error("Error message should contain 'error 2'")
	}
}
