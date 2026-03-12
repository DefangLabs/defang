package acr

import (
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry/v2"
)

// TaskRequest contains parameters for scheduling an ACR task run.
type TaskRequest struct {
	// Image is the container image to run (e.g. "myregistry.azurecr.io/pulumi:latest").
	Image string
	// Command is the command to run in the container.
	Command []string
	// Envs are environment variables passed to the task as template values.
	Envs map[string]string
	// SecretEnvs are secret environment variables (not shown in logs/API responses).
	SecretEnvs map[string]string
	// SourceLocation is the relative path returned by GetBuildSourceUploadURL,
	// or an absolute URL to a tar.gz or git repo. Optional for tasks that don't need source.
	SourceLocation string
	// Timeout is the task timeout (default 1h).
	Timeout time.Duration
}

// RunStatus represents the status of an ACR task run.
type RunStatus struct {
	RunID        string
	Status       armcontainerregistry.RunStatus
	ErrorMessage string
}

// IsTerminal returns true if the run has reached a final state.
func (s RunStatus) IsTerminal() bool {
	switch s.Status {
	case armcontainerregistry.RunStatusSucceeded,
		armcontainerregistry.RunStatusFailed,
		armcontainerregistry.RunStatusCanceled,
		armcontainerregistry.RunStatusError,
		armcontainerregistry.RunStatusTimeout:
		return true
	}
	return false
}

// IsSuccess returns true if the run completed successfully.
func (s RunStatus) IsSuccess() bool {
	return s.Status == armcontainerregistry.RunStatusSucceeded
}
