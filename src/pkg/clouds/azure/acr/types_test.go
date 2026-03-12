package acr

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerregistry/armcontainerregistry/v2"
)

func TestRunStatusHelpers(t *testing.T) {
	tests := []struct {
		status     armcontainerregistry.RunStatus
		isTerminal bool
		isSuccess  bool
	}{
		{armcontainerregistry.RunStatusQueued, false, false},
		{armcontainerregistry.RunStatusStarted, false, false},
		{armcontainerregistry.RunStatusRunning, false, false},
		{armcontainerregistry.RunStatusSucceeded, true, true},
		{armcontainerregistry.RunStatusFailed, true, false},
		{armcontainerregistry.RunStatusCanceled, true, false},
		{armcontainerregistry.RunStatusError, true, false},
		{armcontainerregistry.RunStatusTimeout, true, false},
	}

	for _, tt := range tests {
		s := RunStatus{Status: tt.status}
		if s.IsTerminal() != tt.isTerminal {
			t.Errorf("RunStatus(%s).IsTerminal() = %v, want %v", tt.status, s.IsTerminal(), tt.isTerminal)
		}
		if s.IsSuccess() != tt.isSuccess {
			t.Errorf("RunStatus(%s).IsSuccess() = %v, want %v", tt.status, s.IsSuccess(), tt.isSuccess)
		}
	}
}
