package gcp

import (
	"testing"

	"github.com/DefangLabs/defang/src/pkg/clouds/gcp"
)

type HasName struct {
	name string
}

func (h HasName) Name() string {
	return h.name
}
func (h *HasName) SetName(name string) {
	h.name = name
}

func TestServiceNameRestorer(t *testing.T) {
	services := []string{"service1", "Service2", "SERVICE3", "Service4️⃣", "服务五", "Ṡervicė6"}
	restorer := getServiceNameRestorer(
		services,
		gcp.SafeLabelValue,
		func(n HasName) string { return n.Name() },
		func(n HasName, name string) HasName {
			n.SetName(name)
			return n
		},
	)
	tests := []struct {
		input    string
		expected string
	}{
		{"service1", "service1"},
		{"service2", "Service2"},
		{"service3", "SERVICE3"},
		{"service4-", "Service4️⃣"},
		{"服务五", "服务五"},
		{"ṡervicė6", "Ṡervicė6"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := restorer(HasName{name: test.input})
			if result.Name() != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result.Name())
			}
		})
	}
}
