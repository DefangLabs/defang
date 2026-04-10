package gcp

import (
	"strings"
	"testing"
)

func TestAddComputeEngineInstanceGroupInsertOrPatch(t *testing.T) {
	tests := []struct {
		name     string
		stack    string
		project  string
		etag     string
		services []string
	}{
		{"no args", "", "", "", nil},
		{"with all args", "my-stack", "my-project", "abc123", []string{"svc1", "svc2"}},
		{"with stack only", "my-stack", "", "", nil},
		{"with services only", "", "", "", []string{"svc1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := NewSubscribeQuery()
			q.AddComputeEngineInstanceGroupInsertOrPatch(tt.stack, tt.project, tt.etag, tt.services)
			query := q.GetQuery()

			if !strings.Contains(query, `regionInstanceGroupManagers.(insert|patch)`) {
				t.Errorf("query missing method name filter:\n%v", query)
			}
			if strings.Contains(query, "allInstancesConfig") {
				t.Errorf("query must not contain allInstancesConfig label filters (PATCH requests omit labels):\n%v", query)
			}
			for _, label := range []string{"defang-stack", "defang-project", "defang-etag", "defang-service"} {
				if strings.Contains(query, label) {
					t.Errorf("query must not filter by %q label (labels absent from PATCH request body):\n%v", label, query)
				}
			}
		})
	}
}
