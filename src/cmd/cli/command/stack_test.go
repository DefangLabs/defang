package command

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
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
	mockClient := cliClient.GrpcClient{}
	mockCtrl := &MockFabricControllerClient{
		canIUseResponse: defangv1.CanIUseResponse{},
	}
	mockClient.SetClient(mockCtrl)
	global.Client = &mockClient

	// Set up a fake RootCmd with required flags
	RootCmd = &cobra.Command{Use: "defang"}
	RootCmd.PersistentFlags().StringVarP(&global.Stack.Name, "stack", "s", global.Stack.Name, "stack name")
	RootCmd.PersistentFlags().VarP(&global.Stack.Provider, "provider", "P", "provider")
	RootCmd.PersistentFlags().StringP("project-name", "p", "", "project name")
	RootCmd.PersistentFlags().StringArrayP("file", "f", []string{}, "compose file path(s)")

	// Create stackListCmd with manual RunE to avoid configureLoader call during test
	stackListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Short:   "List existing Defang deployment stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")

			wd, err := os.Getwd()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			// Create a simple loader without using configureLoader to avoid flag issues
			loader := compose.NewLoader()
			projectName, err := loader.LoadProjectName(ctx)
			if err != nil {
				projectName = ""
			}

			sm, err := stacks.NewManager(global.Client, wd, projectName)
			if err != nil {
				return err
			}

			stacks, err := sm.List(ctx)
			if err != nil {
				return err
			}

			if jsonMode {
				jsonData := []byte("[]")
				if len(stacks) > 0 {
					jsonData, err = json.MarshalIndent(stacks, "", "  ")
					if err != nil {
						return err
					}
				}
				_, err = term.Print(string(jsonData) + "\n")
				return err
			}

			if len(stacks) == 0 {
				_, err = term.Infof("No Defang stacks found in the current directory.\n")
				return err
			}

			columns := []string{"Name", "Provider", "Region", "Mode", "DeployedAt"}
			return term.Table(stacks, columns...)
		},
	}
	stackListCmd.Flags().Bool("json", false, "Output in JSON format")

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
					Provider: cliClient.ProviderAWS,
					Region:   "us-west-2",
					Mode:     modes.ModeAffordable,
				},
				{
					Name:     "teststack2",
					Provider: cliClient.ProviderGCP,
					Region:   "us-central1",
					Mode:     modes.ModeBalanced,
				},
			},
			expectOutput: "NAME        PROVIDER  REGION       MODE        DEPLOYEDAT\n" +
				"teststack1  aws       us-west-2    AFFORDABLE  0001-01-01 00:00:00 +0000 UTC  \n" +
				"teststack2  gcp       us-central1  BALANCED    0001-01-01 00:00:00 +0000 UTC  \n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup stacks
			t.Chdir(t.TempDir())
			for _, stack := range tt.stacks {
				stacks.Create(stack)
			}

			buffer := new(bytes.Buffer)
			mockStdin := bytes.NewReader([]byte{})
			MockTerm(t, buffer, mockStdin)

			err := stackListCmd.RunE(stackListCmd, []string{})
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
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
				Mode:     modes.ModeAffordable,
			},
			expectErr: false,
		},
		{
			name: "missing stack name",
			parameters: stacks.StackParameters{
				Name:     "",
				Provider: cliClient.ProviderAWS,
				Region:   "us-west-2",
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
