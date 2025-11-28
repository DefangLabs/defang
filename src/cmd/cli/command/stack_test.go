package command

import (
	"bytes"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/globals"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
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
	var stackListCmd = makeStackListCmd()

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
			expectOutput: "NAME        PROVIDER  REGION       MODE\n" +
				"teststack1  aws       us-west-2    AFFORDABLE  \n" +
				"teststack2  gcp       us-central1  BALANCED    \n",
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
			ni := globals.Config.NonInteractive
			globals.Config.NonInteractive = true
			t.Cleanup(func() { globals.Config.NonInteractive = ni })

			err := stackCreateCmd.RunE(stackCreateCmd, args)
			if (err != nil) != tt.expectErr {
				t.Errorf("RunE() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}
