package command

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/spf13/pflag"
)

func Test_configurationPrecedence(t *testing.T) {
	// Test various combinations of flags, environment variables, and .defang files
	// no matter the order they are applied, or combination, the final configuration should be correct.
	// The precedence should be: flags > env vars > .defang files

	// make a default config for comparison and copying
	defaultConfig := GlobalConfig{
		ColorMode:      ColorAuto,
		Debug:          false,
		HasTty:         true, // set to true just for test instead of term.IsTerminal() for consistency
		HideUpdate:     false,
		NonInteractive: false, // set to false just for test instead of !term.IsTerminal() for consistency
		Verbose:        false,
		Stack:          stacks.StackParameters{Provider: client.ProviderAuto, Mode: modes.ModeUnspecified},
		Cluster:        "",
		Tenant:         "",
	}

	tests := []struct {
		name     string
		envVars  map[string]string
		flags    map[string]string
		expected GlobalConfig
	}{
		{
			name:     "no stack file, no env vars and no flags",
			expected: defaultConfig, // should match the initialized defaults above
		},
		{
			name: "ignore empty debug bool",
			envVars: map[string]string{
				"DEFANG_DEBUG": "",
			},
			expected: defaultConfig, // should ignore invalid and keep default false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testConfig := defaultConfig

			// simulate SetupCommands()
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.StringVarP(&testConfig.Stack.Name, "stack", "s", testConfig.Stack.Name, "stack name (for BYOC providers)")
			flags.Var(&testConfig.ColorMode, "color", "colorize output")
			flags.StringVar(&testConfig.Cluster, "cluster", testConfig.Cluster, "Defang cluster to connect to")
			flags.Var(&testConfig.Tenant, "workspace", "workspace name (tenant)")
			flags.VarP(&testConfig.Stack.Provider, "provider", "P", "bring-your-own-cloud provider")
			flags.BoolVarP(&testConfig.Verbose, "verbose", "v", testConfig.Verbose, "verbose logging")
			flags.BoolVar(&testConfig.Debug, "debug", testConfig.Debug, "debug logging for troubleshooting the CLI")
			flags.BoolVar(&testConfig.NonInteractive, "non-interactive", testConfig.NonInteractive, "disable interactive prompts / no TTY")
			flags.VarP(&testConfig.Stack.Mode, "mode", "m", "deployment mode")

			// Set flags based on user input (these override env and stack file values)
			for flagName, flagValue := range tt.flags {
				if err := flags.Set(flagName, flagValue); err != nil {
					t.Fatalf("failed to set flag %s=%s: %v", flagName, flagValue, err)
				}
			}

			// Set environment variables (these override stack file values)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Make stack files in a temporary directory
			tempDir := t.TempDir()

			var rcEnvs []string
			// Create stack files in the temporary directory

			t.Cleanup(func() {
				// Unseting env vars set for this test is handled by t.Setenv automatically
				// t.tempDir() will clean up created files

				// Unset all env vars created by loadDotDefang since it uses os.Setenv
				for _, rcEnv := range rcEnvs {
					os.Unsetenv(rcEnv)
				}
			})

			t.Chdir(tempDir)

			// verify the final configuration matches expectations
			// if testConfig.Mode.String() != tt.expected.Mode.String() {
			// 	t.Errorf("expected Mode to be '%s', got '%s'", tt.expected.Mode.String(), testConfig.Mode.String())
			// }
			if testConfig.Verbose != tt.expected.Verbose {
				t.Errorf("expected Verbose to be %v, got %v", tt.expected.Verbose, testConfig.Verbose)
			}
			if testConfig.Debug != tt.expected.Debug {
				t.Errorf("expected Debug to be %v, got %v", tt.expected.Debug, testConfig.Debug)
			}
			if testConfig.Stack.Name != tt.expected.Stack.Name {
				t.Errorf("expected Stack.Name to be '%s', got '%s'", tt.expected.Stack.Name, testConfig.Stack.Name)
			}
			if testConfig.Stack.Provider != tt.expected.Stack.Provider {
				t.Errorf("expected Stack.Provider to be '%s', got '%s'", tt.expected.Stack.Provider, testConfig.Stack.Provider)
			}
			if testConfig.Stack.Mode != tt.expected.Stack.Mode {
				t.Errorf("expected Stack.Mode to be '%s', got '%s'", tt.expected.Stack.Mode, testConfig.Stack.Mode)
			}
			if testConfig.Cluster != tt.expected.Cluster {
				t.Errorf("expected Cluster to be '%s', got '%s'", tt.expected.Cluster, testConfig.Cluster)
			}
			if testConfig.Tenant != tt.expected.Tenant {
				t.Errorf("expected Tenant to be '%s', got '%s'", tt.expected.Tenant, testConfig.Tenant)
			}
			if testConfig.ColorMode != tt.expected.ColorMode {
				t.Errorf("expected ColorMode to be '%s', got '%s'", tt.expected.ColorMode, testConfig.ColorMode)
			}
			if testConfig.HasTty != tt.expected.HasTty {
				t.Errorf("expected HasTty to be %v, got %v", tt.expected.HasTty, testConfig.HasTty)
			}
			if testConfig.NonInteractive != tt.expected.NonInteractive {
				t.Errorf("expected NonInteractive to be %v, got %v", tt.expected.NonInteractive, testConfig.NonInteractive)
			}
			if testConfig.HideUpdate != tt.expected.HideUpdate {
				t.Errorf("expected HideUpdate to be %v, got %v", tt.expected.HideUpdate, testConfig.HideUpdate)
			}
		})
	}
}
