package byoc

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
			name:     "Pending operations",
			obj:      mockObj{"testdata/pending.json"},
			expected: `unit-test/pending {defang} (pending "*.unit-test.defang.defang.appValidation" "*.unit-test.defang.defang.appValidation" "*.unit-test.defang.defang.appValidation")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := ParsePulumiStateFile(t.Context(), tt.obj, ".", func(ctx context.Context, bucket, object string) ([]byte, error) {
				return os.ReadFile(filepath.Join(bucket, object))
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
