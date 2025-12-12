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
			name: "project name with underscores",
			bt:   BuildTag{Stack: "stack1", Project: "my_proj_name", Service: "svc", Etag: "123"},
			want: "stack1_my_proj_name_svc_123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.bt.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

//raphaeltm-prod1_zaoconnect_project_app_l1h2a8jygv35

func TestBuildTagParseValid(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want BuildTag
	}{
		{
			name: "3-part tag (backward compatible)",
			tag:  "proj_svc_123",
			want: BuildTag{Stack: "", Project: "proj", Service: "svc", Etag: "123"},
		},
		{
			name: "4-part tag",
			tag:  "stack1_proj_svc_123",
			want: BuildTag{Stack: "stack1", Project: "proj", Service: "svc", Etag: "123"},
		},
		{
			name: "project name with underscores",
			tag:  "stack1_my_proj_name_svc_123",
			want: BuildTag{Stack: "stack1", Project: "my_proj_name", Service: "svc", Etag: "123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bt BuildTag
			err := bt.Parse(tt.tag)
			if err != nil {
				t.Fatalf("Parse() returned error: %v", err)
			}

			if bt != tt.want {
				t.Errorf("Parse() = %+v, want %+v", bt, tt.want)
			}
		})
	}
}
