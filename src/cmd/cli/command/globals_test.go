package command

import (
	"os"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
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
						"DEFANG_MODE":            "AFFORDABLE",
						"DEFANG_VERBOSE":         "false",
						"DEFANG_DEBUG":           "true",
						"DEFANG_STACK":           "from-rc",
						"DEFANG_FABRIC":          "from-rc-cluster",
						"DEFANG_PROVIDER":        "defang",
						"DEFANG_ORG":             "from-rc-org",
						"DEFANG_SOURCE_PLATFORM": "heroku",
						"DEFANG_COLOR":           "never",
						"DEFANG_TTY":             "true",
						"DEFANG_NON_INTERACTIVE": "false",
					},
				},
			},
			envVars: map[string]string{
				"DEFANG_MODE":            "BALANCED",
				"DEFANG_VERBOSE":         "true",
				"DEFANG_DEBUG":           "false",
				"DEFANG_STACK":           "from-env",
				"DEFANG_FABRIC":          "from-env-cluster",
				"DEFANG_PROVIDER":        "gcp",
				"DEFANG_ORG":             "from-env-org",
				"DEFANG_SOURCE_PLATFORM": "heroku",
				"DEFANG_COLOR":           "auto",
			},
			flags: map[string]string{
				"mode":            "HIGH_AVAILABILITY",
				"verbose":         "false",
				"debug":           "true",
				"stack":           "from-flags",
				"cluster":         "from-flags-cluster",
				"provider":        "aws",
				"org":             "from-flags-org",
				"from":            "heroku",
				"color":           "always",
				"non-interactive": "false",
			},
			expected: GlobalConfig{
				Mode:           modes.ModeHighAvailability,
				Verbose:        false,
				Debug:          true,
				Stack:          "from-flags",
				Cluster:        "from-flags-cluster",
				ProviderID:     cliClient.ProviderAWS,
				Org:            "from-flags-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from env override
				NonInteractive: false, // from flags override
			},
		},
		{
			name: "Env overrides rc files when no flags set",
			rcStacks: []stack{
				{
					stackname: "test",
					entries: map[string]string{
						"DEFANG_MODE":            "AFFORDABLE",
						"DEFANG_VERBOSE":         "false",
						"DEFANG_DEBUG":           "true",
						"DEFANG_STACK":           "from-rc",
						"DEFANG_FABRIC":          "from-rc-cluster",
						"DEFANG_PROVIDER":        "defang",
						"DEFANG_ORG":             "from-rc-org",
						"DEFANG_SOURCE_PLATFORM": "heroku",
						"DEFANG_COLOR":           "never",
						"DEFANG_TTY":             "false",
						"DEFANG_NON_INTERACTIVE": "true",
					},
				},
			},
			envVars: map[string]string{
				"DEFANG_MODE":            "BALANCED",
				"DEFANG_VERBOSE":         "true",
				"DEFANG_DEBUG":           "false",
				"DEFANG_STACK":           "from-env",
				"DEFANG_FABRIC":          "from-env-cluster",
				"DEFANG_PROVIDER":        "gcp",
				"DEFANG_ORG":             "from-env-org",
				"DEFANG_SOURCE_PLATFORM": "heroku",
				"DEFANG_COLOR":           "auto",
				"DEFANG_TTY":             "true",
				"DEFANG_NON_INTERACTIVE": "false",
			},
			expected: GlobalConfig{
				Mode:           modes.ModeBalanced,
				Verbose:        true,
				Debug:          false,
				Stack:          "from-env",
				Cluster:        "from-env-cluster",
				ProviderID:     cliClient.ProviderGCP,
				Org:            "from-env-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAuto,
				HasTty:         true,  // from env
				NonInteractive: false, // from env
			},
		},
		{
			name: "RC files used when no env vars or flags",
			rcStacks: []stack{
				{
					stackname: "test",
					entries: map[string]string{
						"DEFANG_MODE":            "AFFORDABLE",
						"DEFANG_VERBOSE":         "true",
						"DEFANG_DEBUG":           "false",
						"DEFANG_STACK":           "from-rc",
						"DEFANG_FABRIC":          "from-rc-cluster",
						"DEFANG_PROVIDER":        "defang",
						"DEFANG_ORG":             "from-rc-org",
						"DEFANG_SOURCE_PLATFORM": "heroku",
						"DEFANG_COLOR":           "always",
						"DEFANG_TTY":             "false",
						"DEFANG_NON_INTERACTIVE": "true",
					},
				},
			},
			expected: GlobalConfig{
				Mode:           modes.ModeAffordable, // RC file values
				Verbose:        true,
				Debug:          false,
				Stack:          "from-rc",
				Cluster:        "from-rc-cluster",
				ProviderID:     cliClient.ProviderDefang,
				Org:            "from-rc-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from rc
				NonInteractive: true,  // from rc
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
			flags.String("cluster", "", "cluster name")
			flags.String("provider", "", "provider name")
			flags.String("org", "", "organization name")
			flags.String("from", "", "source platform")
			flags.String("color", "", "color mode")
			flags.String("non-interactive", "false", "non-interactive mode")

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
			if flagCluster := flags.Lookup("cluster"); flagCluster != nil && flagCluster.Changed {
				config.Cluster = flagCluster.Value.String()
			}
			if flagProvider := flags.Lookup("provider"); flagProvider != nil && flagProvider.Changed {
				config.ProviderID.Set(flagProvider.Value.String())
			}
			if flagOrg := flags.Lookup("org"); flagOrg != nil && flagOrg.Changed {
				config.Org = flagOrg.Value.String()
			}
			if flagFrom := flags.Lookup("from"); flagFrom != nil && flagFrom.Changed {
				config.SourcePlatform.Set(flagFrom.Value.String())
			}
			if flagColor := flags.Lookup("color"); flagColor != nil && flagColor.Changed {
				config.ColorMode.Set(flagColor.Value.String())
			}
			if flagNonInteractive := flags.Lookup("non-interactive"); flagNonInteractive != nil && flagNonInteractive.Changed {
				config.NonInteractive = flagNonInteractive.Value.String() == "true"
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
			if config.Cluster != tt.expected.Cluster {
				t.Errorf("expected Cluster to be '%s', got '%s'", tt.expected.Cluster, config.Cluster)
			}
			if config.ProviderID != tt.expected.ProviderID {
				t.Errorf("expected ProviderID to be '%s', got '%s'", tt.expected.ProviderID, config.ProviderID)
			}
			if config.Org != tt.expected.Org {
				t.Errorf("expected Org to be '%s', got '%s'", tt.expected.Org, config.Org)
			}
			if config.SourcePlatform != tt.expected.SourcePlatform {
				t.Errorf("expected SourcePlatform to be '%s', got '%s'", tt.expected.SourcePlatform, config.SourcePlatform)
			}
			if config.ColorMode != tt.expected.ColorMode {
				t.Errorf("expected ColorMode to be '%s', got '%s'", tt.expected.ColorMode, config.ColorMode)
			}
			if config.HasTty != tt.expected.HasTty {
				t.Errorf("expected HasTty to be %v, got %v", tt.expected.HasTty, config.HasTty)
			}
			if config.NonInteractive != tt.expected.NonInteractive {
				t.Errorf("expected NonInteractive to be %v, got %v", tt.expected.NonInteractive, config.NonInteractive)
			}
		})
	}
}
