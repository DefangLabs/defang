package command

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
)

func TestConfigSetMultiple(t *testing.T) {
	mockService := &mockFabricService{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	t.Chdir("../../../../src/testdata/sanity")

	userinfoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userinfo" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"allTenants":[{"id":"default","name":"Default Workspace"}],
			"userinfo":{"email":"cli@example.com","name":"CLI Tester"}
		}`))
	}))
	t.Cleanup(userinfoServer.Close)

	openAuthClient := auth.OpenAuthClient
	t.Cleanup(func() {
		auth.OpenAuthClient = openAuthClient
	})
	auth.OpenAuthClient = auth.NewClient("testclient", userinfoServer.URL)
	// t.Setenv("DEFANG_ACCESS_TOKEN", "token-123")

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	// prevSts, prevSsm := awsdriver.NewStsFromConfig, awsdriver.NewSsmFromConfig
	// t.Cleanup(func() {
	// 	awsdriver.NewStsFromConfig = prevSts
	// 	awsdriver.NewSsmFromConfig = prevSsm
	// })
	// awsdriver.NewStsFromConfig = func(aws.Config) awsdriver.StsClientAPI { return &awsdriver.MockStsClientAPI{} }
	// awsdriver.NewSsmFromConfig = func(aws.Config) awsdriver.SsmParametersAPI { return &MockSsmClient{} }

	testCases := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{
			name:        "multiple configs with one missing = should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2", "--provider=defang", "--project-name=app"},
			expectedErr: "when setting multiple configs, all must be in KEY=VALUE format",
		},
		{
			name:        "multiple configs with --env should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2=value2", "-e", "--provider=defang", "--project-name=app"},
			expectedErr: "--env is only allowed when setting a single config",
		},
		{
			name:        "multiple configs with --random should error",
			args:        []string{"config", "set", "KEY1=value1", "KEY2=value2", "--random", "--provider=defang", "--project-name=app"},
			expectedErr: "--random is only allowed when setting a single config",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := testCommand(t, tc.args, server.URL)

			if tc.expectedErr != "" {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if !strings.Contains(err.Error(), tc.expectedErr) {
					t.Errorf("expected error message to contain %q, got %q", tc.expectedErr, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
