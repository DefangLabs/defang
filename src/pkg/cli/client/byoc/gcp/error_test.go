package gcp

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestIsValidGcpProjectID(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		wantValid bool
		wantMsg   string
	}{
		{
			name:      "valid project ID",
			projectID: "my-project-123",
			wantValid: true,
		},
		{
			name:      "valid minimum length",
			projectID: "abcdef",
			wantValid: true,
		},
		{
			name:      "too short",
			projectID: "abc",
			wantValid: false,
			wantMsg:   "must be at least 6 characters",
		},
		{
			name:      "too long",
			projectID: "this-is-a-very-long-project-id-that-exceeds-thirty-characters",
			wantValid: false,
			wantMsg:   "must be at most 30 characters",
		},
		{
			name:      "starts with number",
			projectID: "123-project",
			wantValid: false,
			wantMsg:   "must start with a lowercase letter",
		},
		{
			name:      "starts with uppercase",
			projectID: "Project-123",
			wantValid: false,
			wantMsg:   "must start with a lowercase letter",
		},
		{
			name:      "ends with hyphen",
			projectID: "my-project-",
			wantValid: false,
			wantMsg:   "cannot end with a hyphen",
		},
		{
			name:      "contains underscore",
			projectID: "my_project_123",
			wantValid: false,
			wantMsg:   "can only contain lowercase letters, numbers, and hyphens",
		},
		{
			name:      "contains special characters",
			projectID: "my-project@123",
			wantValid: false,
			wantMsg:   "can only contain lowercase letters, numbers, and hyphens",
		},
		{
			name:      "timestamp (from issue)",
			projectID: "2025-10-30T16:36:34.949-07:00",
			wantValid: false,
			wantMsg:   "must start with a lowercase letter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, msg := isValidGcpProjectID(tt.projectID)
			if valid != tt.wantValid {
				t.Errorf("isValidGcpProjectID(%q) valid = %v, want %v", tt.projectID, valid, tt.wantValid)
			}
			if !tt.wantValid && !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("isValidGcpProjectID(%q) message = %q, want to contain %q", tt.projectID, msg, tt.wantMsg)
			}
		})
	}
}

func TestAnnotateGcpError_DeletedProject(t *testing.T) {
	gerr := &googleapi.Error{
		Code:    403,
		Message: "Project cloudbuildtest-468719 has been deleted.",
		Details: []any{
			map[string]any{
				"@type": "type.googleapis.com/google.rpc.ErrorInfo",
				"metadata": map[string]any{
					"consumer": "projects/cloudbuildtest-468719",
					"service":  "serviceusage.googleapis.com",
				},
				"reason": "USER_PROJECT_DENIED",
			},
		},
	}

	annotated := annotateGcpError(gerr)
	
	var deletedErr ErrProjectDeleted
	if !errors.As(annotated, &deletedErr) {
		t.Fatalf("annotateGcpError() = %T, want ErrProjectDeleted", annotated)
	}

	if deletedErr.ProjectID != "cloudbuildtest-468719" {
		t.Errorf("ErrProjectDeleted.ProjectID = %q, want %q", deletedErr.ProjectID, "cloudbuildtest-468719")
	}

	errMsg := annotated.Error()
	if !strings.Contains(errMsg, "gcloud auth application-default login") {
		t.Errorf("error message should suggest running gcloud auth application-default login, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "cloudbuildtest-468719") {
		t.Errorf("error message should mention the project ID, got: %s", errMsg)
	}
}

func TestAnnotateGcpError_InvalidProjectID(t *testing.T) {
	tests := []struct {
		name      string
		error     *googleapi.Error
		projectID string
	}{
		{
			name: "invalid resource ID with timestamp",
			error: &googleapi.Error{
				Code:    400,
				Message: "The resource id 2025-10-30T16:36:34.949-07:00 is invalid.",
				Details: []any{
					map[string]any{
						"@type": "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "RESOURCES_INVALID_RESOURCE_ID",
						"metadata": map[string]any{
							"resource_id": "2025-10-30T16:36:34.949-07:00",
						},
					},
				},
			},
			projectID: "2025-10-30T16:36:34.949-07:00",
		},
		{
			name: "invalid resource ID too short",
			error: &googleapi.Error{
				Code:    400,
				Message: "The resource id abc is invalid.",
				Details: []any{
					map[string]any{
						"@type": "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "RESOURCES_INVALID_RESOURCE_ID",
						"metadata": map[string]any{
							"resource_id": "abc",
						},
					},
				},
			},
			projectID: "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotated := annotateGcpError(tt.error)
			
			var invalidErr ErrInvalidProjectID
			if !errors.As(annotated, &invalidErr) {
				t.Fatalf("annotateGcpError() = %T, want ErrInvalidProjectID", annotated)
			}

			if invalidErr.ProjectID != tt.projectID {
				t.Errorf("ErrInvalidProjectID.ProjectID = %q, want %q", invalidErr.ProjectID, tt.projectID)
			}

			errMsg := annotated.Error()
			if !strings.Contains(errMsg, "GCP project IDs must:") {
				t.Errorf("error message should explain GCP project ID requirements, got: %s", errMsg)
			}
			if !strings.Contains(errMsg, tt.projectID) {
				t.Errorf("error message should mention the invalid project ID, got: %s", errMsg)
			}
		})
	}
}

func TestAnnotateGcpError_OtherErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "nil error",
			err:  nil,
		},
		{
			name: "non-googleapi error",
			err:  errors.New("some other error"),
		},
		{
			name: "googleapi error without special handling",
			err: &googleapi.Error{
				Code:    500,
				Message: "Internal server error",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			annotated := annotateGcpError(tt.err)
			
			if tt.err == nil && annotated != nil {
				t.Errorf("annotateGcpError(nil) = %v, want nil", annotated)
			}

			if tt.err != nil {
				// Should return some error (either briefGcpError or original)
				if annotated == nil {
					t.Errorf("annotateGcpError(%v) = nil, want non-nil", tt.err)
				}
			}
		})
	}
}
