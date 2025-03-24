package compose

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestFixup(t *testing.T) {
	testRunCompose(t, func(t *testing.T, path string) {
		loader := NewLoader(WithPath(path))
		proj, err := loader.LoadProject(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		err = FixupServices(context.Background(), defangv1.SubscriptionTier_HOBBY, client.MockProvider{}, proj, UploadModeIgnore)
		if err != nil {
			t.Fatal(err)
		}

		services := map[string]composeTypes.ServiceConfig{}
		for _, svc := range proj.Services {
			services[svc.Name] = svc
		}

		// Convert the protobuf services to pretty JSON for comparison (YAML would include all the zero values)
		actual, err := json.MarshalIndent(services, "", "  ")
		if err != nil {
			t.Fatal(err)
		}

		if err := compare(actual, path+".fixup"); err != nil {
			t.Error(err)
		}
	})
}

func makeIntPtr(val int) *int {
	return &val
}

func TestServiceDeployFixup(t *testing.T) {
	tests := []struct {
		name     string
		svccfg   *composeTypes.ServiceConfig
		userTier defangv1.SubscriptionTier
		expected string
		replicas *int
	}{
		{
			name:     "Non-PRO tier with nil DeployConfig",
			svccfg:   &composeTypes.ServiceConfig{Name: "test-service"},
			userTier: defangv1.SubscriptionTier_HOBBY,
			expected: "test-service",
			replicas: makeIntPtr(1),
		},
		{
			name: "Non-PRO tier with existing DeployConfig",
			svccfg: &composeTypes.ServiceConfig{
				Name:   "test-service",
				Deploy: &composeTypes.DeployConfig{},
			},
			userTier: defangv1.SubscriptionTier_HOBBY,
			expected: "test-service",
			replicas: makeIntPtr(1),
		},
		{
			name: "Non-PRO tier with existing DeployConfig at 10 replicas",
			svccfg: &composeTypes.ServiceConfig{
				Name: "test-service",
				Deploy: &composeTypes.DeployConfig{
					Replicas: makeIntPtr(10),
				},
			},
			userTier: defangv1.SubscriptionTier_HOBBY,
			expected: "test-service",
			replicas: makeIntPtr(1),
		},
		{
			name: "PRO tier with nil DeployConfig",
			svccfg: &composeTypes.ServiceConfig{
				Name: "test-service",
			},
			userTier: defangv1.SubscriptionTier_PRO,
			expected: "",
			replicas: nil,
		},
		{
			name: "PRO tier with existing DeployConfig",
			svccfg: &composeTypes.ServiceConfig{
				Name: "test-service",
				Deploy: &composeTypes.DeployConfig{
					Replicas: makeIntPtr(10),
				},
			},
			userTier: defangv1.SubscriptionTier_PRO,
			expected: "",
			replicas: makeIntPtr(10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serviceDeployFixup(tt.svccfg, tt.userTier)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}

			if tt.replicas == nil && (tt.svccfg.Deploy == nil || tt.svccfg.Deploy.Replicas == nil) {
				return
			}

			if tt.svccfg.Deploy != nil && tt.svccfg.Deploy.Replicas != nil {
				if *tt.svccfg.Deploy.Replicas != *tt.replicas {
					t.Errorf("expected replicas %d, got %d", tt.replicas, *tt.svccfg.Deploy.Replicas)
				}
			} else {
				t.Errorf("expected replicas %d, got nil", tt.replicas)
			}
		})
	}
}
