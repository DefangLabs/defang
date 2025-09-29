package actions

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
)

var envGcpVarNames = []string{
	"GCP_PROJECT_ID",
	"DEFANG_PROVIDER",
}

var envGcpVars = map[string]string{}

func saveGcpEnvVars() {
	for _, key := range envGcpVarNames {
		var val = os.Getenv(key)
		if val == "" {
			envGcpVars[key] = val
		}
	}
}

func restoreGcpEnvVars() {
	for _, key := range envGcpVarNames {
		if val, ok := envGcpVars[key]; !ok || val == "" {
			os.Unsetenv(key)
			continue
		} else {
			os.Setenv(key, val)
		}
	}
}

func clearGcpEnvVars() {
	for _, key := range envGcpVarNames {
		os.Unsetenv(key)
	}
}

func TestSetGCPBYOCProvider_ValidKeys(t *testing.T) {
	origConnect := common.Connect
	origCheck := common.CheckProviderConfigured
	saveGcpEnvVars()
	defer func() {
		restoreGcpEnvVars()
		common.Connect = origConnect
		common.CheckProviderConfigured = origCheck
	}()
	common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, nil }
	common.CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
		return &client.MockProvider{}, nil
	}

	type testCase struct {
		name       string
		gcpProject string
		connectErr error
		checkErr   error
		wantErr    bool
	}
	tests := []testCase{
		{
			name:       "Valid GCP Project - success",
			gcpProject: "valid-gcp-project",
			connectErr: nil,
			checkErr:   nil,
			wantErr:    false,
		},
		{
			name:       "Valid GCP Project - connect error",
			gcpProject: "valid-gcp-project",
			connectErr: errors.New("connect error"),
			checkErr:   nil,
			wantErr:    true,
		},
		{
			name:       "Valid GCP Project - check error",
			gcpProject: "valid-gcp-project",
			connectErr: nil,
			checkErr:   errors.New("check error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearGcpEnvVars()
			common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, tt.connectErr }
			common.CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
				return &client.MockProvider{}, tt.checkErr
			}
			providerId := client.ProviderID("")
			err := SetGCPByocProvider(t.Context(), &providerId, "test-cluster", tt.gcpProject)
			if tt.wantErr && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got one - %v", err)
			}

			if !tt.wantErr && "gcp" != os.Getenv("DEFANG_PROVIDER") {
				t.Errorf("expected DEFANG_PROVIDER to be %q but got %q", "gcp", os.Getenv("DEFANG_PROVIDER"))
			}
		})
	}
}
