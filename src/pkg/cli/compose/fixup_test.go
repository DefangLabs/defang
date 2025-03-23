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
func TestServiceDeployFixup(t *testing.T) {
	tests := []struct {
		name             string
		svccfg           composeTypes.ServiceConfig
		userTier         defangv1.SubscriptionTier
		expectedReplicas int
		expectedWarnings []string
	}{
		{
			name: "NoDeployConfig",
			svccfg: composeTypes.ServiceConfig{
				Name: "service1",
			},
			userTier:         defangv1.SubscriptionTier_HOBBY,
			expectedReplicas: 1,
			expectedWarnings: []string{},
		},
		{
			name: "NoReplicasConfig",
			svccfg: composeTypes.ServiceConfig{
				Name:   "service2",
				Deploy: &composeTypes.DeployConfig{},
			},
			userTier:         defangv1.SubscriptionTier_HOBBY,
			expectedReplicas: 1,
			expectedWarnings: []string{},
		},
		{
			name: "ReplicasMoreThanOneNonPro",
			svccfg: composeTypes.ServiceConfig{
				Name: "service3",
				Deploy: &composeTypes.DeployConfig{
					Replicas: intPtr(3),
				},
			},
			userTier:         defangv1.SubscriptionTier_HOBBY,
			expectedReplicas: 1,
			expectedWarnings: []string{"service3"},
		},
		{
			name: "ReplicasMoreThanOnePro",
			svccfg: composeTypes.ServiceConfig{
				Name: "service4",
				Deploy: &composeTypes.DeployConfig{
					Replicas: intPtr(3),
				},
			},
			userTier:         defangv1.SubscriptionTier_PRO,
			expectedReplicas: 3,
			expectedWarnings: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := serviceDeployFixup(&tt.svccfg, tt.userTier, []string{})
			if *(tt.svccfg.Deploy.Replicas) != tt.expectedReplicas {
				t.Errorf("expected %d replicas, got %d", tt.expectedReplicas, *tt.svccfg.Deploy.Replicas)
			}
			if !equal(warnings, tt.expectedWarnings) {
				t.Errorf("expected warnings %v, got %v", tt.expectedWarnings, warnings)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
