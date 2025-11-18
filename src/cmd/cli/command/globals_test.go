package command

import (
	"os"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/spf13/pflag"
)

func Test_readGlobals(t *testing.T) {
	t.Chdir("testdata")

	config = GlobalConfig{} // reset globals

	t.Run("OS env beats any .defangrc file", func(t *testing.T) {
		t.Setenv("VALUE", "from OS env")
		config.loadRC("test", nil)
		if v := os.Getenv("VALUE"); v != "from OS env" {
			t.Errorf("expected VALUE to be 'from OS env', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc.test beats .defangrc", func(t *testing.T) {
		config.loadRC("test", nil)
		if v := os.Getenv("VALUE"); v != "from .defangrc.test" {
			t.Errorf("expected VALUE to be 'from .defangrc.test', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc used if no stack", func(t *testing.T) {
		config.loadRC("non-existent-stack", nil)
		if v := os.Getenv("VALUE"); v != "from .defangrc" {
			t.Errorf("expected VALUE to be 'from .defangrc', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})
}

func Test_prorityLoading(t *testing.T) {
	// This is more of an integration test to ensure the loading order is correct
	// when loading from env, rc files, and flags.
	// The precedence should be: flags > env vars > .defangrc files
	t.Chdir("testdata")

	type stack struct {
		stackname string
		entries   map[string]string
	}

	tests := []struct {
		name     string
		rcStacks []stack
		envVars  map[string]string
		flags    map[string]string
		expected GlobalConfig
	}{
		{
			name: "Flags override env and rc files",
			rcStacks: []stack{
				{
					stackname: "test",
					entries: map[string]string{
						"DEFANG_MODE":    "AFFORDABLE",
						"DEFANG_VERBOSE": "false",
						"DEFANG_DEBUG":   "true",
						"DEFANG_STACK":   "from-rc",
					},
				},
			},
			envVars: map[string]string{
				"DEFANG_MODE":    "BALANCED",
				"DEFANG_VERBOSE": "true",
				"DEFANG_DEBUG":   "false",
				"DEFANG_STACK":   "from-env",
			},
			flags: map[string]string{
				"mode":    "HIGH_AVAILABILITY",
				"verbose": "false",
				"debug":   "true",
				"stack":   "from-flags",
			},
			expected: GlobalConfig{
				Mode:    modes.ModeHighAvailability,
				Verbose: false,
				Debug:   true,
				Stack:   "from-flags",
			},
		},
		{
			name: "Env overrides rc files when no flags set",
			rcStacks: []stack{
				{
					stackname: "test",
					entries: map[string]string{
						"DEFANG_MODE":    "AFFORDABLE",
						"DEFANG_VERBOSE": "false",
						"DEFANG_DEBUG":   "true",
						"DEFANG_STACK":   "from-rc",
					},
				},
			},
			envVars: map[string]string{
				"DEFANG_MODE":    "BALANCED",
				"DEFANG_VERBOSE": "true",
				"DEFANG_DEBUG":   "false",
				"DEFANG_STACK":   "from-env",
			},
			expected: GlobalConfig{
				Mode:    modes.ModeBalanced,
				Verbose: true,
				Debug:   false,
				Stack:   "from-env",
			},
		},
		{
			name: "RC files used when no env vars or flags",
			rcStacks: []stack{
				{
					stackname: "test",
					entries: map[string]string{
						"DEFANG_MODE":    "AFFORDABLE",
						"DEFANG_VERBOSE": "true",
						"DEFANG_DEBUG":   "false",
						"DEFANG_STACK":   "from-rc",
					},
				},
			},
			expected: GlobalConfig{
				Mode:    modes.ModeAffordable, // RC file values
				Verbose: true,
				Debug:   false,
				Stack:   "from-rc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset config for each test
			config = GlobalConfig{}

			// Create RC files
			for _, rcStack := range tt.rcStacks {
				filename := ".defangrc." + rcStack.stackname
				defer os.Remove(filename) // Clean up

				f, err := os.Create(filename)
				if err != nil {
					t.Fatalf("failed to create file %s: %v", filename, err)
				}

				// Write as environment file format (KEY=VALUE)
				for key, value := range rcStack.entries {
					if _, err := f.WriteString(key + "=" + value + "\n"); err != nil {
						t.Fatalf("failed to write to file %s: %v", filename, err)
					}
				}
				f.Close()
			}

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create flag set to simulate command line flags
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("mode", "", "deployment mode")
			flags.Bool("verbose", false, "verbose output")
			flags.Bool("debug", false, "debug output")
			flags.String("stack", "", "stack name")

			// Set flags if provided
			for flagName, flagValue := range tt.flags {
				if err := flags.Set(flagName, flagValue); err != nil {
					t.Fatalf("failed to set flag %s=%s: %v", flagName, flagValue, err)
				}
			}

			// Load configuration in the correct order
			stackName := ""
			if len(tt.rcStacks) > 0 {
				stackName = tt.rcStacks[0].stackname
			}
			if flagStack := flags.Lookup("stack"); flagStack != nil && flagStack.Changed {
				stackName = flagStack.Value.String()
			}

			// This simulates the actual loading sequence
			config.loadRC(stackName, flags)

			// Apply flags to config (simulate what happens in the actual CLI)
			if flagMode := flags.Lookup("mode"); flagMode != nil && flagMode.Changed {
				config.Mode, _ = modes.Parse(flagMode.Value.String())
			}
			if flagVerbose := flags.Lookup("verbose"); flagVerbose != nil && flagVerbose.Changed {
				config.Verbose = flagVerbose.Value.String() == "true"
			}
			if flagDebug := flags.Lookup("debug"); flagDebug != nil && flagDebug.Changed {
				config.Debug = flagDebug.Value.String() == "true"
			}
			if flagStack := flags.Lookup("stack"); flagStack != nil && flagStack.Changed {
				config.Stack = flagStack.Value.String()
			}

			// Verify the final configuration matches expectations
			if config.Mode.String() != tt.expected.Mode.String() {
				t.Errorf("expected Mode to be '%s', got '%s'", tt.expected.Mode.String(), config.Mode.String())
			}
			if config.Verbose != tt.expected.Verbose {
				t.Errorf("expected Verbose to be %v, got %v", tt.expected.Verbose, config.Verbose)
			}
			if config.Debug != tt.expected.Debug {
				t.Errorf("expected Debug to be %v, got %v", tt.expected.Debug, config.Debug)
			}
			if config.Stack != tt.expected.Stack {
				t.Errorf("expected Stack to be '%s', got '%s'", tt.expected.Stack, config.Stack)
			}
		})
	}
}
