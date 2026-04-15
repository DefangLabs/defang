package command

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

// mockFabricClientWithStacks is a minimal FabricClient mock for testing stackExists.
type mockFabricClientWithStacks struct {
	client.MockFabricClient
	existingStacks  []*defangv1.Stack
	listStacksErr   error
	expectedProject string
}

func (m mockFabricClientWithStacks) GetStack(_ context.Context, req *defangv1.GetStackRequest) (*defangv1.GetStackResponse, error) {
	if m.listStacksErr != nil {
		return nil, m.listStacksErr
	}
	if m.expectedProject != "" && req.Project != m.expectedProject {
		return &defangv1.GetStackResponse{}, nil
	}
	for _, s := range m.existingStacks {
		if s.Name == req.Stack {
			return &defangv1.GetStackResponse{Stack: s}, nil
		}
	}
	return &defangv1.GetStackResponse{}, nil
}

const testComposeYaml = `services:
  web:
    image: nginx`

func MockTerm(t *testing.T, stdout *bytes.Buffer, stdin *bytes.Reader) {
	t.Helper()
	oldTerm := term.DefaultTerm
	term.DefaultTerm = term.NewTerm(
		&FakeStdin{stdin},
		&FakeStdout{stdout},
		new(bytes.Buffer),
	)
	t.Cleanup(func() {
		term.DefaultTerm = oldTerm
	})
}

func TestStackListCmd(t *testing.T) {
	// Save original RootCmd and restore after test
	origRootCmd := RootCmd
	origClient := global.Client
	t.Cleanup(func() {
		RootCmd = origRootCmd
		global.Client = origClient
	})

	// Set up a mock client (shared, but stacksToList is updated per subtest)
	mockClient := client.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetFabricClient(mockCtrl)
	global.Client = &mockClient

	downStackFile := []byte("DEFANG_PROVIDER=aws\nAWS_REGION=us-test-1\nDEFANG_MODE=affordable")
	upStackFile := []byte("DEFANG_PROVIDER=gcp\nGOOGLE_REGION=us-central1\nDEFANG_MODE=balanced")

	tests := []struct {
		name         string
		localStacks  []stacks.Parameters
		remoteStacks []*defangv1.Stack
		cmdArgs      []string
		expectOutput string
		containsAll  []string
		containsNone []string
	}{
		{
			name:         "no stacks present",
			localStacks:  []stacks.Parameters{},
			cmdArgs:      []string{"list"},
			expectOutput: " * No Defang stacks found in the current directory.\n",
		},
		{
			name: "multiple stacks present",
			localStacks: []stacks.Parameters{
				{
					Name:     "teststack1",
					Provider: client.ProviderAWS,
					Region:   "us-test-2",
					Mode:     modes.ModeAffordable,
				},
				{
					Name:     "teststack2",
					Provider: client.ProviderGCP,
					Region:   "us-central1",
					Mode:     modes.ModeBalanced,
				},
			},
			cmdArgs: []string{"list"},
			expectOutput: "NAME        DEFAULT  PROVIDER  REGION       ACCOUNT  MODE        DEPLOYEDAT\n" +
				"teststack1           aws       us-test-2             AFFORDABLE    \n" +
				"teststack2           gcp       us-central1           BALANCED      \n",
		},
		{
			name: "down stack hidden by default",
			remoteStacks: []*defangv1.Stack{
				{Name: "downstack", Status: defangv1.StackStatus_STACK_STATUS_DOWN, StackFile: downStackFile},
			},
			cmdArgs:      []string{"list"},
			expectOutput: " * All stacks in the current directory are down.\n",
		},
		{
			name: "down stack shown with --all",
			remoteStacks: []*defangv1.Stack{
				{Name: "downstack", Status: defangv1.StackStatus_STACK_STATUS_DOWN, StackFile: downStackFile},
			},
			cmdArgs:     []string{"list", "--all"},
			containsAll: []string{"downstack"},
		},
		{
			name: "mixed stacks, down hidden without --all",
			remoteStacks: []*defangv1.Stack{
				{Name: "upstack", Status: defangv1.StackStatus_STACK_STATUS_UP, StackFile: upStackFile},
				{Name: "downstack", Status: defangv1.StackStatus_STACK_STATUS_DOWN, StackFile: downStackFile},
			},
			cmdArgs:      []string{"list"},
			containsAll:  []string{"upstack"},
			containsNone: []string{"downstack"},
		},
		{
			name: "mixed stacks, all shown with --all",
			remoteStacks: []*defangv1.Stack{
				{Name: "upstack", Status: defangv1.StackStatus_STACK_STATUS_UP, StackFile: upStackFile},
				{Name: "downstack", Status: defangv1.StackStatus_STACK_STATUS_DOWN, StackFile: downStackFile},
			},
			cmdArgs:     []string{"list", "--all"},
			containsAll: []string{"upstack", "downstack"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Recreate RootCmd and stackListCmd per subtest so flag state is fresh
			RootCmd = &cobra.Command{Use: "defang"}
			RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name")
			RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")
			RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
			RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "compose file path(s)")
			RootCmd.AddCommand(makeStackListCmd())

			// Setup stacks
			t.Chdir(t.TempDir())
			// create a compose file so stackListCmd doesn't error out
			os.WriteFile(
				"compose.yaml",
				[]byte(`services:
  web:
    image: nginx`),
				os.FileMode(0644),
			)
			for _, stack := range tt.localStacks {
				stacks.CreateInDirectory(".", stack)
			}
			mockCtrl.stacksToList = tt.remoteStacks

			buffer := new(bytes.Buffer)
			mockStdin := bytes.NewReader([]byte{})
			MockTerm(t, buffer, mockStdin)

			RootCmd.SetArgs(tt.cmdArgs)
			err := RootCmd.Execute()
			assert.NoError(t, err)
			if tt.expectOutput != "" {
				assert.Equal(t, tt.expectOutput, buffer.String())
			}
			for _, s := range tt.containsAll {
				assert.Contains(t, buffer.String(), s)
			}
			for _, s := range tt.containsNone {
				assert.NotContains(t, buffer.String(), s)
			}
		})
	}
}

func TestNonInteractiveStackNewCmd(t *testing.T) {
	origClient := global.Client
	origNI := global.NonInteractive
	global.NonInteractive = true
	t.Cleanup(func() {
		global.Client = origClient
		global.NonInteractive = origNI
	})

	tests := []struct {
		name           string
		parameters     stacks.Parameters
		existingStacks []*defangv1.Stack
		expectErr      bool
	}{
		{
			name: "valid parameters",
			parameters: stacks.Parameters{
				Name:     "teststack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Mode:     modes.ModeAffordable,
			},
			existingStacks: []*defangv1.Stack{},
			expectErr:      false,
		},
		{
			name: "missing stack name",
			parameters: stacks.Parameters{
				Name:     "",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Mode:     modes.ModeAffordable,
			},
			existingStacks: []*defangv1.Stack{},
			expectErr:      true,
		},
		{
			name: "stack already exists",
			parameters: stacks.Parameters{
				Name:     "existingstack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Mode:     modes.ModeAffordable,
			},
			existingStacks: []*defangv1.Stack{{Name: "existingstack", Project: ""}},
			expectErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			os.WriteFile("compose.yaml", []byte(testComposeYaml), 0644)

			global.Client = mockFabricClientWithStacks{existingStacks: tt.existingStacks}

			// Recreate the cmd each subtest so flags reset cleanly
			stackCreateCmd := makeStackNewCmd()
			stackCreateCmd.SetContext(t.Context())
			stackCreateCmd.Flags().Set("region", tt.parameters.Region)

			err := stackCreateCmd.RunE(stackCreateCmd, []string{tt.parameters.Name})
			if (err != nil) != tt.expectErr {
				t.Errorf("RunE() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestLoadStackEnv(t *testing.T) {
	tests := []struct {
		name        string
		parameters  stacks.Parameters
		expectedEnv map[string]string
	}{
		{
			name: "AWS parameters",
			parameters: stacks.Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
				Variables: map[string]string{
					"AWS_PROFILE": "default",
				},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "aws",
				"AWS_REGION":      "us-west-2",
				"AWS_PROFILE":     "default",
				"DEFANG_MODE":     "affordable",
			},
		},
		{
			name: "GCP parameters",
			parameters: stacks.Parameters{
				Provider: client.ProviderGCP,
				Region:   "us-central1",
				Mode:     modes.ModeBalanced,
				Variables: map[string]string{
					"GCP_PROJECT_ID": "my-gcp-project",
					"DEFANG_PREFIX":  "test",
					"DEFANG_SUFFIX":  "dev",
				},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "gcp",
				"GOOGLE_REGION":   "us-central1",
				"GCP_PROJECT_ID":  "my-gcp-project",
				"DEFANG_MODE":     "balanced",
				"DEFANG_PREFIX":   "test",
				"DEFANG_SUFFIX":   "dev",
			},
		},
		{
			name: "With prefix and suffix",
			parameters: stacks.Parameters{
				Provider: client.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
				Variables: map[string]string{
					"AWS_PROFILE":   "default",
					"DEFANG_PREFIX": "test",
					"DEFANG_SUFFIX": "dev",
				},
			},
			expectedEnv: map[string]string{
				"DEFANG_PROVIDER": "aws",
				"AWS_REGION":      "us-west-2",
				"AWS_PROFILE":     "default",
				"DEFANG_MODE":     "affordable",
				"DEFANG_PREFIX":   "test",
				"DEFANG_SUFFIX":   "dev",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any existing env vars that might interfere with the test
			for key := range tt.expectedEnv {
				os.Unsetenv(key)
			}

			t.Cleanup(func() {
				// Clean up environment variables after test
				for key := range tt.expectedEnv {
					os.Unsetenv(key)
				}
			})

			err := stacks.LoadStackEnv(tt.parameters, true)
			if err != nil {
				t.Fatalf("LoadStackEnv() error = %v", err)
			}

			for key, expectedValue := range tt.expectedEnv {
				if value := os.Getenv(key); value != expectedValue {
					t.Errorf("Environment variable %s = %s; want %s", key, value, expectedValue)
				}
			}
		})
	}
}

func TestStackExists(t *testing.T) {
	origClient := global.Client
	t.Cleanup(func() { global.Client = origClient })

	tests := []struct {
		name            string
		stackName       string
		existingStacks  []*defangv1.Stack
		listStacksErr   error
		expectedProject string
		want            bool
		wantErr         bool
	}{
		{
			name:            "stack exists",
			stackName:       "mystack",
			existingStacks:  []*defangv1.Stack{{Name: "mystack"}},
			expectedProject: "testproject",
			want:            true,
		},
		{
			name:            "stack not found among others",
			stackName:       "mystack",
			existingStacks:  []*defangv1.Stack{{Name: "otherstack"}, {Name: "anotherstack"}},
			expectedProject: "testproject",
			want:            false,
		},
		{
			name:            "no stacks exist",
			stackName:       "mystack",
			existingStacks:  []*defangv1.Stack{},
			expectedProject: "testproject",
			want:            false,
		},
		{
			name:      "empty stack name always returns false",
			stackName: "",
			existingStacks: []*defangv1.Stack{
				{Name: "mystack"},
				{Name: ""},
			},
			expectedProject: "testproject",
			want:            false,
		},
		{
			name:            "wrong project returns false",
			stackName:       "mystack",
			existingStacks:  []*defangv1.Stack{{Name: "mystack"}},
			expectedProject: "otherproject",
			want:            false,
		},
		{
			name:          "GetStack error is propagated",
			stackName:     "mystack",
			listStacksErr: context.DeadlineExceeded,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			global.Client = mockFabricClientWithStacks{
				existingStacks:  tt.existingStacks,
				listStacksErr:   tt.listStacksErr,
				expectedProject: tt.expectedProject,
			}

			got, err := stackExists(t.Context(), "testproject", tt.stackName)
			if (err != nil) != tt.wantErr {
				t.Errorf("stackExists() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("stackExists() = %v, want %v", got, tt.want)
			}
		})
	}
}
