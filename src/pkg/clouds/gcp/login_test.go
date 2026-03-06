package gcp

import "testing"

func TestParseWIFProvider(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantProject  string
		wantPool     string
		wantProvider string
		wantErr      bool
	}{
		{
			name:         "valid provider string",
			input:        "//iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/defang-github/providers/github-actions",
			wantProject:  "123456789012",
			wantPool:     "defang-github",
			wantProvider: "github-actions",
		},
		{
			name:         "valid provider with suffix",
			input:        "//iam.googleapis.com/projects/my-project/locations/global/workloadIdentityPools/my-pool/providers/my-provider",
			wantProject:  "my-project",
			wantPool:     "my-pool",
			wantProvider: "my-provider",
		},
		{
			name:    "too few segments",
			input:   "//iam.googleapis.com/projects/123/locations/global",
			wantErr: true,
		},
		{
			name:    "wrong keyword workloadIdentityPools",
			input:   "//iam.googleapis.com/projects/123/locations/global/wrongKeyword/pool/providers/provider",
			wantErr: true,
		},
		{
			name:    "wrong keyword providers",
			input:   "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/wrongKeyword/provider",
			wantErr: true,
		},
		{
			name:    "wrong keyword projects",
			input:   "//iam.googleapis.com/wrongKeyword/123/locations/global/workloadIdentityPools/pool/providers/provider",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			project, pool, provider, err := parseWIFProvider(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseWIFProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if project != tt.wantProject {
					t.Errorf("project = %q, want %q", project, tt.wantProject)
				}
				if pool != tt.wantPool {
					t.Errorf("pool = %q, want %q", pool, tt.wantPool)
				}
				if provider != tt.wantProvider {
					t.Errorf("provider = %q, want %q", provider, tt.wantProvider)
				}
			}
		})
	}
}
