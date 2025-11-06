package compose

import (
	"fmt"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
)

func TestGetHealthCheckPathAndPort(t *testing.T) {
	tests := []struct {
		healthcheck  *types.HealthCheckConfig
		expectedPath string
		expectedPort int
	}{
		{
			healthcheck: &types.HealthCheckConfig{
				Test: []string{"CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"},
			},
			expectedPath: "/health",
			expectedPort: 8080,
		},
		{
			healthcheck: &types.HealthCheckConfig{
				Test: []string{"CMD", "curl", "-f", "http://localhost/status"},
			},
			expectedPath: "/status",
			expectedPort: 80,
		},
		{
			healthcheck: &types.HealthCheckConfig{
				Test: []string{"CMD", "curl", "-f", "https://example.com/ping && echo 'http://localhost:1234'"},
			},
			expectedPath: "/",
			expectedPort: 1234,
		},
		{
			healthcheck:  nil,
			expectedPath: "/",
			expectedPort: 80,
		},
		{
			healthcheck: &types.HealthCheckConfig{
				Test: nil,
			},
			expectedPath: "/",
			expectedPort: 80,
		},
		{
			healthcheck: &types.HealthCheckConfig{
				Test: []string{},
			},
			expectedPath: "/",
			expectedPort: 80,
		},
		{
			healthcheck: &types.HealthCheckConfig{
				Test: []string{"CMD-SHELL", "some invalid command"},
			},
			expectedPath: "/",
			expectedPort: 80,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.healthcheck), func(t *testing.T) {
			path, port := GetHealthCheckPathAndPort(tt.healthcheck)
			if path != tt.expectedPath {
				t.Errorf("expected path %q, got %q", tt.expectedPath, path)
			}
			if port != tt.expectedPort {
				t.Errorf("expected port %d, got %d", tt.expectedPort, port)
			}
		})
	}
}
