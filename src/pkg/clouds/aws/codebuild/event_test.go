package codebuild

import (
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestParseCodebuildEvent(t *testing.T) {
	tests := []struct {
		name          string
		logEntry      *defangv1.LogEntry
		expected      Event
		expectedState defangv1.ServiceState
	}{
		{
			name: "Basic CodeBuild Event",
			logEntry: &defangv1.LogEntry{
				Message: "Running on CodeBuild",
				Service: "my-service",
				Etag:    "etag-123",
				Host:    "host-abc",
			},
			expected: &CodebuildEvent{
				message: "Running on CodeBuild",
				service: "my-service",
				etag:    "etag-123",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_ACTIVATING,
			},
			expectedState: defangv1.ServiceState_BUILD_ACTIVATING,
		},
		{
			name: "Build Failed Event",
			logEntry: &defangv1.LogEntry{
				Message: "Phase complete: BUILD State: FAILED",
				Service: "failing-service",
				Etag:    "etag-456",
				Host:    "host-def",
			},
			expected: &CodebuildEvent{
				message: "Phase complete: BUILD State: FAILED",
				service: "failing-service",
				etag:    "etag-456",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_FAILED,
			},
			expectedState: defangv1.ServiceState_BUILD_FAILED,
		},
		{
			name: "Build Succeeded Event",
			logEntry: &defangv1.LogEntry{
				Message: "Phase complete: UPLOAD_ARTIFACTS State: SUCCEEDED",
				Service: "successful-service",
				Etag:    "etag-789",
				Host:    "host-ghi",
			},
			expected: &CodebuildEvent{
				message: "Phase complete: UPLOAD_ARTIFACTS State: SUCCEEDED",
				service: "successful-service",
				etag:    "etag-789",
				host:    "codebuild",
				state:   defangv1.ServiceState_DEPLOYMENT_PENDING,
			},
			expectedState: defangv1.ServiceState_DEPLOYMENT_PENDING,
		},
		{
			name: "Unknown Event",
			logEntry: &defangv1.LogEntry{
				Message: "Some unrelated log message",
				Service: "unknown-service",
				Etag:    "etag-000",
				Host:    "host-xyz",
			},
			expected: &CodebuildEvent{
				message: "Some unrelated log message",
				service: "unknown-service",
				etag:    "etag-000",
				host:    "codebuild",
				state:   defangv1.ServiceState_NOT_SPECIFIED,
			},
			expectedState: defangv1.ServiceState_NOT_SPECIFIED,
		},
		{
			name: "Install Phase Event",
			logEntry: &defangv1.LogEntry{
				Message: "Entering phase INSTALL",
				Service: "install-service",
				Etag:    "etag-111",
				Host:    "host-install",
			},
			expected: &CodebuildEvent{
				message: "Entering phase INSTALL",
				service: "install-service",
				etag:    "etag-111",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_RUNNING,
			},
			expectedState: defangv1.ServiceState_BUILD_RUNNING,
		},
		{
			name: "Pre-Build Phase Event",
			logEntry: &defangv1.LogEntry{
				Message: "Entering phase PRE_BUILD",
				Service: "prebuild-service",
				Etag:    "etag-222",
				Host:    "host-prebuild",
			},
			expected: &CodebuildEvent{
				message: "Entering phase PRE_BUILD",
				service: "prebuild-service",
				etag:    "etag-222",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_RUNNING,
			},
			expectedState: defangv1.ServiceState_BUILD_RUNNING,
		},
		{
			name: "Build Phase Event",
			logEntry: &defangv1.LogEntry{
				Message: "Entering phase BUILD",
				Service: "build-service",
				Etag:    "etag-333",
				Host:    "host-build",
			},
			expected: &CodebuildEvent{
				message: "Entering phase BUILD",
				service: "build-service",
				etag:    "etag-333",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_RUNNING,
			},
			expectedState: defangv1.ServiceState_BUILD_RUNNING,
		},
		{
			name: "Post-Build Phase Event",
			logEntry: &defangv1.LogEntry{
				Message: "Entering phase POST_BUILD",
				Service: "postbuild-service",
				Etag:    "etag-444",
				Host:    "host-postbuild",
			},
			expected: &CodebuildEvent{
				message: "Entering phase POST_BUILD",
				service: "postbuild-service",
				etag:    "etag-444",
				host:    "codebuild",
				state:   defangv1.ServiceState_BUILD_STOPPING,
			},
			expectedState: defangv1.ServiceState_BUILD_STOPPING,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := ParseCodebuildEvent(tt.logEntry)
			if event.Service() != tt.expected.Service() {
				t.Errorf("expected service %s, got %s", tt.expected.Service(), event.Service())
			}
			if event.Etag() != tt.expected.Etag() {
				t.Errorf("expected etag %s, got %s", tt.expected.Etag(), event.Etag())
			}
			if event.Host() != tt.expected.Host() {
				t.Errorf("expected host %s, got %s", tt.expected.Host(), event.Host())
			}
			if event.State() != tt.expected.State() {
				t.Errorf("expected state %v, got %v", tt.expected.State(), event.State())
			}
		})
	}
}
