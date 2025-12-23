package command

import (
	"bytes"
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
	defer func() {
		RootCmd = origRootCmd
		global.Client = origClient
	}()

	// Set up a mock client
	mockClient := client.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetFabricClient(mockCtrl)
	global.Client = &mockClient

	// Set up a fake RootCmd with required flags
	RootCmd = &cobra.Command{Use: "defang"}
	RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name")
	RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "compose file path(s)")

	// Create stackListCmd with manual RunE to avoid configureLoader call during test
	var stackListCmd = makeStackListCmd()

	// Add stackListCmd as a child of RootCmd
	RootCmd.AddCommand(stackListCmd)

	tests := []struct {
		name         string
		stacks       []stacks.StackParameters
		expectOutput string
	}{
		{
			name:         "no stacks present",
			stacks:       []stacks.StackParameters{},
			expectOutput: " * No Defang stacks found in the current directory.\n",
		},
		{
			name: "multiple stacks present",
			stacks: []stacks.StackParameters{
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
			expectOutput: "NAME        PROVIDER  REGION       MODE        DEPLOYEDAT\n" +
				"teststack1  aws       us-test-2    AFFORDABLE  0001-01-01 00:00:00 +0000 UTC  \n" +
				"teststack2  gcp       us-central1  BALANCED    0001-01-01 00:00:00 +0000 UTC  \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			for _, stack := range tt.stacks {
				stacks.Create(stack)
			}

			buffer := new(bytes.Buffer)
			mockStdin := bytes.NewReader([]byte{})
			MockTerm(t, buffer, mockStdin)

			RootCmd.SetArgs([]string{"list"})
			err := RootCmd.Execute()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectOutput, buffer.String())
		})
	}
}

func TestNonInteractiveStackNewCmd(t *testing.T) {
	var stackCreateCmd = makeStackNewCmd()

	tests := []struct {
		name       string
		parameters stacks.StackParameters
		expectErr  bool
	}{
		{
			name: "valid parameters",
			parameters: stacks.StackParameters{
				Name:     "teststack",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr: false,
		},
		{
			name: "missing stack name",
			parameters: stacks.StackParameters{
				Name:     "",
				Provider: client.ProviderAWS,
				Region:   "us-test-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			args := []string{tt.parameters.Name}
			// Set flags
			stackCreateCmd.Flags().Set("region", tt.parameters.Region)

			// Mock non-interactive mode
			ni := global.NonInteractive
			global.NonInteractive = true
			t.Cleanup(func() { global.NonInteractive = ni })

			err := stackCreateCmd.RunE(stackCreateCmd, args)
			if (err != nil) != tt.expectErr {
				t.Errorf("RunE() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestLoadParameters(t *testing.T) {
	params := map[string]string{
		"DEFANG_PROVIDER": "aws",
		"AWS_REGION":      "us-west-2",
		"AWS_PROFILE":     "default",
		"DEFANG_MODE":     "AFFORDABLE",
	}

	// Clear any existing env vars that might interfere with the test
	os.Unsetenv("DEFANG_PROVIDER")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("DEFANG_MODE")

	defer func() {
		// Clean up environment variables after test
		os.Unsetenv("DEFANG_PROVIDER")
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("AWS_PROFILE")
		os.Unsetenv("DEFANG_MODE")
	}()

	err := stacks.LoadParameters(params, true)
	if err != nil {
		t.Fatalf("LoadParameters() error = %v", err)
	}

	assert.Equal(t, "aws", os.Getenv("DEFANG_PROVIDER"))
	assert.Equal(t, "us-west-2", os.Getenv("AWS_REGION"))
	assert.Equal(t, "default", os.Getenv("AWS_PROFILE"))
	assert.Equal(t, "AFFORDABLE", os.Getenv("DEFANG_MODE"))
}
