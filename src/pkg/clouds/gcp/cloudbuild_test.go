package gcp

import (
	"testing"
)

func TestBuildTagString(t *testing.T) {
	tests := []struct {
		name string
		bt   BuildTag
		want string
	}{
		{
			name: "with stack",
			bt:   BuildTag{Stack: "stack1", Project: "proj", Service: "svc", Etag: "123"},
			want: "stack1_proj_svc_123",
		},
		{
			name: "without stack",
			bt:   BuildTag{Project: "proj", Service: "svc", Etag: "123"},
			want: "proj_svc_123",
		},
		{
			name: "service name with underscores",
			bt:   BuildTag{Stack: "stack1", Project: "my-proj-name", Service: "svc_name", Etag: "123"},
			want: "stack1_my-proj-name_svc_name_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagStr := tt.bt.String()
			if tagStr != tt.want {
				t.Errorf("String() = %q, want %q", tagStr, tt.want)
			}
			var parsed BuildTag
			err := parsed.Parse([]string{tagStr})
			if err != nil {
				t.Fatalf("Parse() returned error: %v", err)
			}

			if parsed != tt.bt {
				t.Errorf("Parse() = %+v, want %+v", parsed, tt.want)
			}
		})
	}
}
