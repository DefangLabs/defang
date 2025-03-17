package byoc

import "testing"

func TestGetPulumiBackend(t *testing.T) {
	const stateUrl = "s3://my-bucket"
	tests := []struct {
		name      string
		stateEnv  string
		pat       string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "Pulumi Cloud w/ PAT",
			stateEnv:  "pulumi-cloud",
			pat:       "pul_1234asdf",
			wantKey:   "PULUMI_ACCESS_TOKEN",
			wantValue: "pul_1234asdf",
			wantErr:   false,
		},
		{
			name:      "Pulumi Cloud w/o PAT",
			stateEnv:  "pulumi-cloud",
			pat:       "",
			wantKey:   "PULUMI_ACCESS_TOKEN",
			wantValue: "",
			wantErr:   true,
		},
		{
			name:      "Default w/ PAT",
			stateEnv:  "",
			pat:       "pul_1234asdf",
			wantKey:   "PULUMI_BACKEND_URL",
			wantValue: stateUrl,
			wantErr:   false,
		},
		{
			name:      "Default w/o PAT",
			stateEnv:  "",
			pat:       "",
			wantKey:   "PULUMI_BACKEND_URL",
			wantValue: stateUrl,
			wantErr:   false,
		},
		{
			name:      "Custom w/ PAT",
			stateEnv:  "gs://my-bucket",
			pat:       "pul_1234asdf",
			wantKey:   "PULUMI_BACKEND_URL",
			wantValue: "gs://my-bucket",
			wantErr:   false,
		},
		{
			name:      "Custom w/o PAT",
			stateEnv:  "gs://my-bucket",
			pat:       "",
			wantKey:   "PULUMI_BACKEND_URL",
			wantValue: "gs://my-bucket",
			wantErr:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DefangPulumiBackend = tt.stateEnv
			t.Setenv("PULUMI_ACCESS_TOKEN", tt.pat)
			k, v, err := GetPulumiBackend(stateUrl)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetPulumiBackend() error = %v, wantErr %v", err, tt.wantErr)
			}
			if k != tt.wantKey {
				t.Errorf("GetPulumiBackend() key = %v, want %v", k, tt.wantKey)
			}
			if v != tt.wantValue {
				t.Errorf("GetPulumiBackend() value = %v, want %v", v, tt.wantValue)
			}
		})
	}
}
