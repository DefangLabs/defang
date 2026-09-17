package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type mockObj struct {
	name string
}

func (m mockObj) Name() string {
	return m.name
}

func (m mockObj) Size() int64 {
	b, _ := os.Stat(m.name)
	return b.Size()
}

func TestParsePulumiStateFile(t *testing.T) {
	tests := []struct {
		name     string
		obj      mockObj
		expected string
	}{
		{
			name: "Empty stack",
			obj:  mockObj{"testdata/empty.json"},
		},
		{
			name:     "AWS",
			obj:      mockObj{"testdata/aws.json"},
			expected: "unit-test/aws {defang}",
		},
		{
			name:     "GCP",
			obj:      mockObj{"testdata/gcp.json"},
			expected: "unit-test/gcp {t1234567}",
		},
		{
			name:     "Azure",
			obj:      mockObj{"testdata/azure.json"},
			expected: "unit-test/azure {t1234567}",
		},
		{
			name:     "Pending operations",
			obj:      mockObj{"testdata/pending.json"},
			expected: `unit-test/pending {defang} (pending "*.unit-test.defang.defang.appValidation" "*.unit-test.defang.defang.appValidation" "*.unit-test.defang.defang.appValidation")`,
		},
		{
			name: "Unsupported version",
			obj:  mockObj{"testdata/unsupported-version.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := ParsePulumiStateFile(t.Context(), tt.obj, func(ctx context.Context, object string) ([]byte, error) {
				return os.ReadFile(filepath.Join(".", object))
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.expected == "" {
				if state != nil {
					t.Fatalf("expected nil state, got %v", state)
				}
			} else {
				if state == nil {
					t.Fatalf("expected non-nil state")
				}
				if state.String() != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, state.String())
				}
			}
		})
	}
}

func TestPulumiStateDescendants(t *testing.T) {
	state := PulumiState{Resources: []Resource{
		{URN: "project", Type: "defang-azure:index:Project"},
		{URN: "service", Parent: "project", Type: "defang-azure:index:Service"},
		{URN: "vmss", Parent: "service", Type: "azure-native:compute:VirtualMachineScaleSet"},
		{URN: "extension", Parent: "vmss", Type: "azure-native:compute:VirtualMachineScaleSetExtension"},
		{URN: "other", Parent: "project", Type: "azure-native:app:ContainerApp"},
	}}

	got := state.Descendants("service")
	if len(got) != 2 || got[0].URN != "vmss" || got[1].URN != "extension" {
		t.Fatalf("Descendants(service) = %+v", got)
	}
}
