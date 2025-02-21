package byoc

import (
	"fmt"
	"testing"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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
