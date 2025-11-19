package command

import (
	"fmt"
	"os"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/spf13/pflag"
)

func Test_readGlobals(t *testing.T) {
	t.Chdir("testdata")

	var testConfig GlobalConfig
	testConfig = GlobalConfig{} // reset globals

	t.Run("OS env beats any .defangrc file", func(t *testing.T) {
		t.Setenv("VALUE", "from OS env")
		testConfig.loadRC("test", nil)
		if v := os.Getenv("VALUE"); v != "from OS env" {
			t.Errorf("expected VALUE to be 'from OS env', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc.test beats .defangrc", func(t *testing.T) {
		testConfig.loadRC("test", nil)
		if v := os.Getenv("VALUE"); v != "from .defangrc.test" {
			t.Errorf("expected VALUE to be 'from .defangrc.test', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defangrc used if no stack", func(t *testing.T) {
		testConfig.loadRC("non-existent-stack", nil)
		if v := os.Getenv("VALUE"); v != "from .defangrc" {
			t.Errorf("expected VALUE to be 'from .defangrc', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})
}

func Test_priorityLoading(t *testing.T) {
	// This test to ensure the loading order is correct
	// when loading from env, rc files, and flags.
	// The precedence should be: flags > env vars > .defangrc files

	// make a default config for for comparison and copying
	newDefaultConfig := func() GlobalConfig {
		return GlobalConfig{
			ColorMode:      ColorAuto,
			Debug:          false,
			HasTty:         true, // set to true just for test instead of term.IsTerminal() for consistency
			HideUpdate:     false,
			Mode:           modes.ModeUnspecified,
			NonInteractive: false, // set to false just for test instead of !term.IsTerminal() for consistency
			ProviderID:     cliClient.ProviderAuto,
			SourcePlatform: migrate.SourcePlatformUnspecified,
			Verbose:        false,
			Stack:          "",
			Cluster:        getCluster(),
			Org:            "",
		}
	}

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
						"DEFANG_TTY":             "false",
						"DEFANG_NON_INTERACTIVE": "true",
						"DEFANG_HIDE_UPDATE":     "true",
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
				"DEFANG_TTY":             "false",
				"DEFANG_HIDE_UPDATE":     "false",
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
				HideUpdate:     false, // from env override (env false beats rc true)
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
				"DEFANG_HIDE_UPDATE":     "false",
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
				HideUpdate:     false, // from env (env overrides rc)
			},
		},
		{
			name: "RC file used when no env vars or flags",
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
						"DEFANG_HIDE_UPDATE":     "true",
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
				HideUpdate:     true,  // from rc
			},
		},
		{
			name:     "no rc file, no env vars and no flags results in defaults",
			expected: newDefaultConfig(), // should match the initialized defaults above
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testConfig := newDefaultConfig()

			// simulate SetupCommands()
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.StringVarP(&testConfig.Stack, "stack", "s", testConfig.Stack, "stack name (for BYOC providers)")
			flags.Var(&testConfig.ColorMode, "color", "colorize output")
			flags.StringVar(&testConfig.Cluster, "cluster", testConfig.Cluster, "Defang cluster to connect to")
			flags.StringVar(&testConfig.Org, "org", testConfig.Org, "override GitHub organization name (tenant)")
			flags.VarP(&testConfig.ProviderID, "provider", "P", "bring-your-own-cloud provider")
			flags.BoolVarP(&testConfig.Verbose, "verbose", "v", testConfig.Verbose, "verbose logging")
			flags.BoolVar(&testConfig.Debug, "debug", testConfig.Debug, "debug logging for troubleshooting the CLI")
			flags.BoolVar(&testConfig.NonInteractive, "non-interactive", testConfig.NonInteractive, "disable interactive prompts / no TTY")
			flags.Var(&testConfig.SourcePlatform, "from", "the platform from which to migrate the project")
			flags.VarP(&testConfig.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))

			tempDir := t.TempDir()
			originalDir, _ := os.Getwd()
			os.Chdir(tempDir)

			filenames := []string{".defangrc"}
			rcEnvs := []string{}
			// Create RC files in the temporary directory
			for _, rcStack := range tt.rcStacks {
				filename := ".defangrc." + rcStack.stackname
				filenames = append(filenames, filename)

				f, err := os.Create(filename)
				if err != nil {
					t.Fatalf("failed to create file %s: %v", filename, err)
				}

				// Write as environment file format (KEY=VALUE)
				for key, value := range rcStack.entries {
					if _, err := f.WriteString(key + "=" + value + "\n"); err != nil {
						t.Fatalf("failed to write to file %s: %v", filename, err)
					}
					rcEnvs = append(rcEnvs, key)
				}
				f.Close()
			}

			// Set environment variables (these override RC file values)
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Set flags based on user input (these override env and RC file values)
			for flagName, flagValue := range tt.flags {
				if err := flags.Set(flagName, flagValue); err != nil {
					t.Fatalf("failed to set flag %s=%s: %v", flagName, flagValue, err)
				}
			}

			stackName := ""
			if len(tt.rcStacks) > 0 {
				stackName = tt.rcStacks[0].stackname
				flagStack := flags.Lookup("stack")
				if flagStack != nil && flagStack.Changed {
					stackName = flagStack.Value.String()
				}
			}

			// This simulates the actual loading sequence
			testConfig.loadRC(stackName, flags)

			// Verify the final configuration matches expectations
			if testConfig.Mode.String() != tt.expected.Mode.String() {
				t.Errorf("expected Mode to be '%s', got '%s'", tt.expected.Mode.String(), testConfig.Mode.String())
			}
			if testConfig.Verbose != tt.expected.Verbose {
				t.Errorf("expected Verbose to be %v, got %v", tt.expected.Verbose, testConfig.Verbose)
			}
			if testConfig.Debug != tt.expected.Debug {
				t.Errorf("expected Debug to be %v, got %v", tt.expected.Debug, testConfig.Debug)
			}
			if testConfig.Stack != tt.expected.Stack {
				t.Errorf("expected Stack to be '%s', got '%s'", tt.expected.Stack, testConfig.Stack)
			}
			if testConfig.Cluster != tt.expected.Cluster {
				t.Errorf("expected Cluster to be '%s', got '%s'", tt.expected.Cluster, testConfig.Cluster)
			}
			if testConfig.ProviderID != tt.expected.ProviderID {
				t.Errorf("expected ProviderID to be '%s', got '%s'", tt.expected.ProviderID, testConfig.ProviderID)
			}
			if testConfig.Org != tt.expected.Org {
				t.Errorf("expected Org to be '%s', got '%s'", tt.expected.Org, testConfig.Org)
			}
			if testConfig.SourcePlatform != tt.expected.SourcePlatform {
				t.Errorf("expected SourcePlatform to be '%s', got '%s'", tt.expected.SourcePlatform, testConfig.SourcePlatform)
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

			// cleanup to ensure complete test isolation
			t.Cleanup(func() {
				// Unset all environment variables
				for key, _ := range tt.envVars {
					os.Unsetenv(key)
				}

				// Unset all RC env vars
				for _, rcEnv := range rcEnvs {
					os.Unsetenv(rcEnv)
				}

				// Remove temp directory and all its contents
				os.RemoveAll(tempDir)

				// Remove any .defangrc* files that might have been created
				for _, rcFile := range filenames {
					os.Remove(rcFile)
				}

				// Restore original directory after test
				os.Chdir(originalDir)
			})
		})
	}
}
