package byoc

import (
	"context"
	"os"
	"testing"
)

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

func TestDebugPulumiGolang(t *testing.T) {
	pulumiDir := os.Getenv("DEFANG_PULUMI_DIR")
	if pulumiDir == "" {
		t.Skip("DEFANG_PULUMI_DIR not set; skipping local Pulumi Golang test")
	}

	// REGION=us-central1 GCP_PROJECT_ID=jordan-project-463223 PULUMI_COPILOT=false PROJECT=nodejs-http PULUMI_BACKEND_URL=gs://defang-cd-tk0ak2pj5jwi PULUMI_CONFIG_PASSPHRASE=asdf PULUMI_SKIP_UPDATE_CHECK=true DEFANG_PREFIX=Defang DEFANG_STATE_URL=gs://defang-cd-tk0ak2pj5jwi GCP_PROJECT=jordan-project-463223 STACK=beta DEFANG_ORG=defang DEFANG_DEBUG=1 DEFANG_MODE=mode_unspecified DOMAIN=beta.nodejs-http.jordanstephens.defang.app DEFANG_JSON= REGION=us-central1 DEFANG_ETAG=iqzc5ldbyhsg
	envs := []string{
		"REGION=us-central1",
		"GCP_PROJECT_ID=jordan-project-463223",
		"PULUMI_COPILOT=false",
		"PROJECT=golang-http",
		"PULUMI_BACKEND_URL=gs://defang-cd-tk0ak2pj5jwi",
		"PULUMI_CONFIG_PASSPHRASE=asdf",
		"PULUMI_SKIP_UPDATE_CHECK=true",
		"DEFANG_PREFIX=Defang",
		"DEFANG_STATE_URL=gs://defang-cd-tk0ak2pj5jwi",
		"GCP_PROJECT=jordan-project-463223",
		"STACK=beta",
		"DEFANG_ORG=defang",
		"DEFANG_DEBUG=1",
		"DEFANG_MODE=mode_unspecified",
		"DOMAIN=beta.nodejs-http.jordanstephens.defang.app",
		"DEFANG_JSON=",
		"REGION=us-central1",
		"DEFANG_ETAG=iqzc5ldbyhsg",
	}

	err := DebugPulumiGolang(context.Background(), envs, "up", "CqIBCgwKA2FwcDIFCLgXGAESNGFwcC0tMzAwMC5iZXRhLm5vZGVqcy1odHRwLmpvcmRhbnN0ZXBoZW5zLmRlZmFuZy5hcHAaC25vZGVqcy1odHRwIgxpcXpjNWxkYnloc2cqDEJVSUxEX1FVRVVFREouYXBwLmJldGEubm9kZWpzLWh0dHAuam9yZGFuc3RlcGhlbnMuZGVmYW5nLmFwcHgBkAEBIugEbmFtZTogbm9kZWpzLWh0dHAKc2VydmljZXM6CiAgYXBwOgogICAgYnVpbGQ6CiAgICAgIGNvbnRleHQ6IGdzOi8vZGVmYW5nLWNkLXRrMGFrMnBqNWp3aS91cGxvYWRzL3NoYTI1Ni03R2Z4eG51dmEwWkclMkJrYkxKVGI0SFVmZndFNGkwTDByT0JwT29Zdm03RVklM0QudGFyLmd6CiAgICAgIGRvY2tlcmZpbGU6IERvY2tlcmZpbGUKICAgIGRlcGxveToKICAgICAgcmVzb3VyY2VzOgogICAgICAgIHJlc2VydmF0aW9uczoKICAgICAgICAgIG1lbW9yeTogIjI2ODQzNTQ1NiIKICAgIGhlYWx0aGNoZWNrOgogICAgICB0ZXN0OgogICAgICAgIC0gQ01ECiAgICAgICAgLSBjdXJsCiAgICAgICAgLSAtZgogICAgICAgIC0gaHR0cDovL2xvY2FsaG9zdDozMDAwLwogICAgbmV0d29ya3M6CiAgICAgIGRlZmF1bHQ6IG51bGwKICAgIHBvcnRzOgogICAgICAtIG1vZGU6IGluZ3Jlc3MKICAgICAgICB0YXJnZXQ6IDMwMDAKICAgICAgICBwdWJsaXNoZWQ6ICIzMDAwIgogICAgICAgIHByb3RvY29sOiB0Y3AKICAgICAgICBhcHBfcHJvdG9jb2w6IGh0dHAKICAgIHJlc3RhcnQ6IHVubGVzcy1zdG9wcGVkCm5ldHdvcmtzOgogIGRlZmF1bHQ6CiAgICBuYW1lOiBub2RlanMtaHR0cF9kZWZhdWx0CiqDAWluZGV4LmRvY2tlci5pby9kZWZhbmdpby9jZDpwdWJsaWMtZ2NwLXYwLjYuMC04ODQtZzRlYzEwNDc3QHNoYTI1Njo1YWRhYzFmZWI1NzJmZDkzMzQwY2VkNWFlZGQ4NTE2ZDdkNTRiZDQwOWJiYWExNTRiYzY4YTQxMGJhZDg3OTM1")
	if err != nil {
		t.Fatal(err)
	}
}
