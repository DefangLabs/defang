package actions

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/mcp/common"
)

var awsEnvVarNames = []string{
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_REGION",
	"AWS_PROFILE",
	"DEFANG_PROVIDER",
}

var awsEnvVars = map[string]string{}

func saveAwsEnvVars() {
	for _, key := range awsEnvVarNames {
		var val = os.Getenv(key)
		if val == "" {
			awsEnvVars[key] = val
		}
	}
}

func restoreAwsEnvVars() {
	for _, key := range awsEnvVarNames {
		if val, ok := awsEnvVars[key]; !ok || val == "" {
			os.Unsetenv(key)
			continue
		} else {
			os.Setenv(key, val)
		}
	}
}

func clearAwsEnvVars() {
	for _, key := range awsEnvVarNames {
		os.Unsetenv(key)
	}
}

func TestSetAWSBYOCProvider_ValidKeys(t *testing.T) {
	validAwsID := "ABIA12345678901234"
	origConnect := common.Connect
	origCheck := common.CheckProviderConfigured
	saveAwsEnvVars()
	defer func() {
		restoreAwsEnvVars()
		common.Connect = origConnect
		common.CheckProviderConfigured = origCheck
	}()
	type testCase struct {
		name       string
		awsId      string
		awsSecret  string
		region     string
		connectErr error
		checkErr   error
		wantErr    bool
	}
	tests := []testCase{
		// valid AWS ID
		{
			name:       "Valid AWS Key - success",
			awsId:      validAwsID,
			awsSecret:  "awsSecret",
			region:     "us-test-2",
			connectErr: nil,
			checkErr:   nil,
			wantErr:    false,
		},
		{
			name:       "Valid AWS Key - connect fail",
			awsId:      validAwsID,
			awsSecret:  "awsSecret",
			region:     "us-test-2",
			connectErr: errors.New("connect error"),
			checkErr:   nil,
			wantErr:    true,
		},
		// valid AWS ID
		{
			name:       "Valid AWS Key - check error",
			awsId:      validAwsID,
			awsSecret:  "awsSecret",
			region:     "us-test-2",
			connectErr: nil,
			checkErr:   errors.New("check error"),
			wantErr:    true,
		},
		// invalid AWS ID
		{
			name:      "Invalid AWS Key - success",
			awsId:     "awsId",
			awsSecret: "awsSecret",
			region:    "us-test-2",
			wantErr:   false,
		},
		{
			name:       "Invalid AWS Key - connect fail",
			awsId:      "awsId",
			awsSecret:  "awsSecret",
			region:     "us-test-2",
			connectErr: errors.New("connect error"),
			checkErr:   nil,
			wantErr:    true,
		},
		// valid AWS ID
		{
			name:       "Invalid AWS Key - check error",
			awsId:      "awsId",
			awsSecret:  "awsSecret",
			region:     "us-test-2",
			connectErr: nil,
			checkErr:   errors.New("check error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAwsEnvVars()
			common.Connect = func(ctx context.Context, cluster string) (*client.GrpcClient, error) { return nil, tt.connectErr }
			common.CheckProviderConfigured = func(ctx context.Context, fabric client.FabricClient, providerId client.ProviderID, s string, i int) (client.Provider, error) {
				return &client.MockProvider{}, tt.checkErr
			}
			providerId := client.ProviderID("")
			err := SetAWSByocProvider(t.Context(), &providerId, "test-cluster", tt.awsId, tt.awsSecret, tt.region)
			if tt.wantErr && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantErr && err != nil {
				t.Errorf("expected no error but got one - %v", err)
			}

			if IsValidAWSKey(tt.awsId) {
				if tt.awsId != os.Getenv("AWS_ACCESS_KEY_ID") {
					t.Errorf("expected AWS_ACCESS_KEY_ID to be %q but got %q", tt.awsId, os.Getenv("AWS_ACCESS_KEY_ID"))
				}
				if tt.awsSecret != os.Getenv("AWS_SECRET_ACCESS_KEY") {
					t.Errorf("expected AWS_SECRET_ACCESS_KEY to be %q but got %q", tt.awsSecret, os.Getenv("AWS_SECRET_ACCESS_KEY"))
				}
			}

			if tt.region != os.Getenv("AWS_REGION") {
				t.Errorf("expected AWS_REGION to be %q but got %q", tt.region, os.Getenv("AWS_REGION"))
			}

			if !tt.wantErr && "aws" != os.Getenv("DEFANG_PROVIDER") {
				t.Errorf("expected DEFANG_PROVIDER to be %q but got %q", "aws", os.Getenv("DEFANG_PROVIDER"))
			}
		})
	}
}
