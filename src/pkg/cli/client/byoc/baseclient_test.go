package byoc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestTopologicalSort(t *testing.T) {
	tests := []map[string][]string{
		{"a": {"b", "c"}, "b": {"c"}, "c": {}}, // Simple
		{"a": {"b", "c"}, "d": {"e", "f"}, "g": {"h", "i"}, "b": {}, "c": {}, "e": {}, "f": {}, "h": {}, "i": {}}, // Multiple roots
		{"a": {"b", "c"}, "b": {"g"}, "c": {}, "d": {"e", "f"}, "e": {"g"}, "f": {}, "g": {}},                     // Multiple roots with shared dependency
		{"a": {"b"}, "b": {"c"}, "c": {"d"}, "d": {"e"}, "e": {"f"}, "f": {}},                                     // Long chain
		{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}, "d": {}},                                                        // Diamond
		{ // Cross dependency
			"a": {"b"}, "b": {"c"}, "c": {"d"}, "d": {"e", "j"}, "e": {"f"}, "f": {},
			"g": {"h"}, "h": {"i"}, "i": {"d"}, "j": {"k"}, "k": {},
		},
		{"a": {"b", "c"}, "b": {"c"}}, // Excluded service
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt), func(t *testing.T) {
			m := make(map[string]*Node)
			for name, deps := range tt {
				m[name] = &Node{Name: name, Deps: deps, ServiceInfo: &defangv1.ServiceInfo{Service: &defangv1.Service{Name: name}}}
			}
			sorted := topologicalSort(m)
			if len(sorted) != len(tt) {
				t.Errorf("sorted array missing service info: expected %d, got %d", len(tt), len(sorted))
			}

			posMap := make(map[string]int)
			for i, si := range sorted {
				posMap[si.Service.Name] = i
			}

			for _, si := range sorted {
				for _, dep := range m[si.Service.Name].Deps {
					if posMap[si.Service.Name] < posMap[dep] {
						t.Errorf("dependency %q is not before %q", dep, si.Service.Name)
					}
				}
			}
		})
	}
}

type mockGetServiceInfosProvider struct {
	client.Provider
	*ByocBaseClient
}

func (m mockGetServiceInfosProvider) UpdateServiceInfo(ctx context.Context, serviceInfo *defangv1.ServiceInfo, projectName, delegateDomain string, service composeTypes.ServiceConfig) error {
	serviceInfo.ZoneId = "test-zone-id"
	return nil
}

func (m mockGetServiceInfosProvider) GetProjectUpdate(context.Context, string) (*defangv1.ProjectUpdate, error) {
	return nil, nil
}

func NewMockGetServiceInfosProvider() *mockGetServiceInfosProvider {
	p := &mockGetServiceInfosProvider{}
	p.ByocBaseClient = NewByocBaseClient(context.Background(), "test-tenant", p)
	return p
}

// The array order has to be 3, 2, 1 because of the dependencies
var expectedServiceInfosJson = `[
  {
    "service": {
      "name": "service3"
    },
    "project": "test-project",
    "etag": "test-etag",
    "status": "UPDATE_QUEUED",
    "zone_id": "test-zone-id",
    "state": 7,
    "name": "service3"
  },
  {
    "service": {
      "name": "service2"
    },
    "project": "test-project",
    "etag": "test-etag",
    "status": "UPDATE_QUEUED",
    "zone_id": "test-zone-id",
    "state": 7,
    "name": "service2"
  },
  {
    "service": {
      "name": "service1"
    },
    "project": "test-project",
    "etag": "test-etag",
    "status": "UPDATE_QUEUED",
    "zone_id": "test-zone-id",
    "state": 7,
    "name": "service1"
  }
]`

func TestGetServiceInfos(t *testing.T) {
	testProvider := NewMockGetServiceInfosProvider()

	serviceInfos, err := testProvider.GetServiceInfos(context.Background(), "test-project", "test-delegate-domain", "test-etag",
		map[string]composeTypes.ServiceConfig{
			"service1": {
				Name:      "service1",
				Image:     "test-image1",
				DependsOn: map[string]composeTypes.ServiceDependency{"service2": {}, "service3": {}},
			},
			"service2": {
				Name:      "service2",
				Image:     "test-image2",
				DependsOn: map[string]composeTypes.ServiceDependency{"service3": {}},
			},
			"service3": {
				Name:  "service3",
				Image: "test-image3",
			},
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := json.MarshalIndent(serviceInfos, "", "  ")
	if err != nil {
		t.Fatalf("unexpected error while marshalling serviceInfos to json: %v", err)
	}
	if string(b) != expectedServiceInfosJson {
		t.Errorf("expected %s, got %s", expectedServiceInfosJson, string(b))
	}
}

func TestGetServiceInfosWithTestData(t *testing.T) {
	var tests = map[string]string{
		"a->b,c, b->c": "../../../../testdata/dependson/compose.yaml",
	}

	for name, path := range tests {
		t.Run(name, func(t *testing.T) {
			loader := compose.NewLoader(compose.WithPath(path))
			proj, err := loader.LoadProject(context.Background())
			if err != nil {
				t.Fatal(err)
			}

			testProvider := NewMockGetServiceInfosProvider()
			serviceInfos, err := testProvider.GetServiceInfos(context.Background(), proj.Name, "test-delegate-domain", "test-etag", proj.Services)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			indexMap := make(map[string]int)
			for i, si := range serviceInfos {
				indexMap[si.Service.Name] = i
			}

			for _, si := range serviceInfos {
				for dep := range proj.Services[si.Service.Name].DependsOn {
					if indexMap[si.Service.Name] < indexMap[dep] {
						t.Errorf("dependency %q is not before %q", dep, si.Service.Name)
					}
				}
			}
		})
	}
}
