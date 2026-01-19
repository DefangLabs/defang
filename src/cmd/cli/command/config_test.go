package command

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/protos/io/defang/v1/defangv1connect"
)

func resetConfigSetFlags() {
	configSetCmd.Flags().Set("env", "false")
	configSetCmd.Flags().Set("random", "false")
	configSetCmd.Flags().Set("env-file", "")
	RootCmd.PersistentFlags().Set("dry-run", "false")
}

func TestConfigSetFlagConflicts(t *testing.T) {
	mockService := &mockFabricService{}
	_, handler := defangv1connect.NewFabricControllerHandler(mockService)
	t.Chdir("../../../../src/testdata/sanity")

	// Set up environment variables for --env tests
	t.Setenv("KEY1", "value1")
	t.Setenv("KEY2", "value2")

	// Create a temp env file for --env-file tests
	tempDir := t.TempDir()
	envFilePath := filepath.Join(tempDir, "test.env")
	if err := os.WriteFile(envFilePath, []byte("TEST_KEY=test_value\n"), 0644); err != nil {
		t.Fatalf("failed to create test env file: %v", err)
	}

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

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	testCases := []struct {
		name        string
		args        []string
		expectedErr string
	}{
		{
			name:        "cannot use --random with --env",
			args:        []string{"config", "set", "KEY1", "KEY2", "-e", "--random", "--provider=defang", "--project-name=app"},
			expectedErr: "cannot use --random with --env",
		},
		{
			name:        "cannot use --random with --env-file",
			args:        []string{"config", "set", "KEY1", "--random", "--env-file=" + envFilePath, "--provider=defang", "--project-name=app"},
			expectedErr: "cannot use --random with --env-file",
		},
		{
			name:        "cannot use --env with --env-file",
			args:        []string{"config", "set", "KEY1", "-e", "--env-file=" + envFilePath, "--provider=defang", "--project-name=app"},
			expectedErr: "cannot use --env with --env-file",
		},
		{
			name:        "no args with no flags",
			args:        []string{"config", "set", "--provider=defang", "--project-name=app"},
			expectedErr: "provide CONFIG argument or use --env-file to read from a file",
		},
		{
			name:        "too many args with no flags",
			args:        []string{"config", "set", "KEY1", "KEY2", "KEY3", "--provider=defang", "--project-name=app"},
			expectedErr: "too many arguments; provide a single CONFIG or use --env, --random, or --env-file",
		},
		{
			name: "valid use of --env",
			args: []string{"config", "set", "KEY1", "KEY2", "--env", "--provider=defang", "--project-name=app"},
		},
		{
			name: "valid use of --random",
			args: []string{"config", "set", "KEY1", "KEY2", "--random", "--provider=defang", "--project-name=app"},
		},
		{
			name: "valid use of --env-file",
			args: []string{"config", "set", "--env-file=" + envFilePath, "--provider=defang", "--project-name=app"},
		},
		{
			name: "valid use of KEY=VALUE format",
			args: []string{"config", "set", "KEY1=somevalue", "--provider=defang", "--project-name=app"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(resetConfigSetFlags)

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
