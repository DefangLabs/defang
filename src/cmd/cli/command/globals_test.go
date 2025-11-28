package command

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/spf13/pflag"
)

func Test_readGlobals(t *testing.T) {
	testConfig := GlobalConfig{}

	t.Run("OS env beats any .defang file", func(t *testing.T) {
		t.Chdir("testdata/with-stack")
		t.Setenv("VALUE", "from OS env")
		err := testConfig.loadDotDefang("test")
		if err != nil {
			t.Fatalf("%v", err)
		}
		if v := os.Getenv("VALUE"); v != "from OS env" {
			t.Errorf("expected VALUE to be 'from OS env', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defang/test beats .defang", func(t *testing.T) {
		t.Chdir("testdata/with-stack")
		err := testConfig.loadDotDefang("test")
		if err != nil {
			t.Fatalf("%v", err)
		}
		if v := os.Getenv("VALUE"); v != "from .defang/test" {
			t.Errorf("expected VALUE to be 'from .defang/test', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run(".defang used if no stack", func(t *testing.T) {
		t.Chdir("testdata/no-stack")
		err := testConfig.loadDotDefang("")
		if err != nil {
			t.Fatalf("%v", err)
		}
		if v := os.Getenv("VALUE"); v != "from .defang" {
			t.Errorf("expected VALUE to be 'from .defang', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run("incorrect stackname used if no stack", func(t *testing.T) {
		err := testConfig.loadDotDefang("non-existent-stack")
		if err == nil {
			t.Fatalf("this test should fail for non-existent stack: %v", err)
		}
	})
}

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
		SourcePlatform: migrate.SourcePlatformUnspecified,
		Verbose:        false,
		Stack: stacks.StackParameters{
			Provider: cliClient.ProviderAuto,
			Mode:     modes.ModeUnspecified,
		},
		Cluster: "",
		Org:     "",
	}

	type stack struct {
		stackname string
		entries   map[string]string
	}

	tests := []struct {
		name         string
		rcStack      stack
		createRCFile bool
		envVars      map[string]string
		flags        map[string]string
		expected     GlobalConfig
	}{
		{
			name:         "Flags override env and env file",
			createRCFile: true,
			rcStack: stack{
				stackname: "test",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "false",
					"DEFANG_DEBUG":           "true",
					"DEFANG_STACK":           "from-env",
					"DEFANG_FABRIC":          "from-env-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-org",
					"DEFANG_SOURCE_PLATFORM": "heroku",
					"DEFANG_COLOR":           "never",
					"DEFANG_TTY":             "false",
					"DEFANG_NON_INTERACTIVE": "true",
					"DEFANG_HIDE_UPDATE":     "true",
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
				Verbose:        false,
				Debug:          true,
				Stack:          stacks.StackParameters{Name: "from-flags", Provider: cliClient.ProviderAWS, Mode: modes.ModeHighAvailability},
				Cluster:        "from-flags-cluster",
				Org:            "from-flags-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from env override
				NonInteractive: false, // from flags override
				HideUpdate:     false, // from env override (env false beats env true)
			},
		},
		{
			name:         "Env overrides env files when no flags set",
			createRCFile: true,
			rcStack: stack{
				stackname: "test",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "false",
					"DEFANG_DEBUG":           "true",
					"DEFANG_STACK":           "from-env",
					"DEFANG_FABRIC":          "from-env-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-org",
					"DEFANG_SOURCE_PLATFORM": "heroku",
					"DEFANG_COLOR":           "never",
					"DEFANG_TTY":             "false",
					"DEFANG_NON_INTERACTIVE": "true",
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
				Verbose:        true,
				Debug:          false,
				Stack:          stacks.StackParameters{Name: "from-env", Provider: cliClient.ProviderGCP, Mode: modes.ModeBalanced},
				Cluster:        "from-env-cluster",
				Org:            "from-env-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAuto,
				HasTty:         true,  // from env
				NonInteractive: false, // from env
				HideUpdate:     false, // from env (env overrides env)
			},
		},
		{
			name:         "env file used when no env vars or flags set",
			createRCFile: true,
			rcStack: stack{
				stackname: "test",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "true",
					"DEFANG_DEBUG":           "false",
					"DEFANG_STACK":           "from-env",
					"DEFANG_FABRIC":          "from-env-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-org",
					"DEFANG_SOURCE_PLATFORM": "heroku",
					"DEFANG_COLOR":           "always",
					"DEFANG_TTY":             "false",
					"DEFANG_NON_INTERACTIVE": "true",
					"DEFANG_HIDE_UPDATE":     "true",
				},
			},
			expected: GlobalConfig{
				Verbose:        true,
				Debug:          false,
				Stack:          stacks.StackParameters{Name: "from-env", Provider: cliClient.ProviderDefang, Mode: modes.ModeAffordable},
				Cluster:        "from-env-cluster",
				Org:            "from-env-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from env
				NonInteractive: true,  // from env
				HideUpdate:     true,  // from env
			},
		},
		{
			name:         "env file with no values used when no env vars or flags set",
			createRCFile: true,
			rcStack: stack{
				stackname: "test",
			},
			expected: defaultConfig,
		},
		{
			name:         "default .defang name, when no env vars or flags",
			createRCFile: true,
			rcStack: stack{
				stackname: "",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "true",
					"DEFANG_DEBUG":           "false",
					"DEFANG_STACK":           "from-env",
					"DEFANG_FABRIC":          "from-env-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-org",
					"DEFANG_SOURCE_PLATFORM": "heroku",
					"DEFANG_COLOR":           "always",
					"DEFANG_TTY":             "false",
					"DEFANG_NON_INTERACTIVE": "true",
					"DEFANG_HIDE_UPDATE":     "true",
				},
			},
			expected: GlobalConfig{
				Verbose:        true,
				Debug:          false,
				Stack:          stacks.StackParameters{Name: "from-env", Provider: cliClient.ProviderDefang, Mode: modes.ModeAffordable},
				Cluster:        "from-env-cluster",
				Org:            "from-env-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from env
				NonInteractive: true,  // from env
				HideUpdate:     true,  // from env
			},
		},
		{
			name:         "default .defang name and no values, when no env vars or flags",
			createRCFile: true,
			rcStack: stack{
				stackname: "",
			},
			expected: defaultConfig,
		},
		{
			name:         "no env file, no env vars and no flags",
			createRCFile: false,
			expected:     defaultConfig, // should match the initialized defaults above
		},
		{
			name:         "ignore empty debug bool",
			createRCFile: false,
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
			flags.StringVar(&testConfig.Org, "org", testConfig.Org, "override GitHub organization name (tenant)")
			flags.VarP(&testConfig.Stack.Provider, "provider", "P", "bring-your-own-cloud provider")
			flags.BoolVarP(&testConfig.Verbose, "verbose", "v", testConfig.Verbose, "verbose logging")
			flags.BoolVar(&testConfig.Debug, "debug", testConfig.Debug, "debug logging for troubleshooting the CLI")
			flags.BoolVar(&testConfig.NonInteractive, "non-interactive", testConfig.NonInteractive, "disable interactive prompts / no TTY")
			flags.Var(&testConfig.SourcePlatform, "from", "the platform from which to migrate the project")
			flags.VarP(&testConfig.Stack.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))

			// Set flags based on user input (these override env and env file values)
			for flagName, flagValue := range tt.flags {
				if err := flags.Set(flagName, flagValue); err != nil {
					t.Fatalf("failed to set flag %s=%s: %v", flagName, flagValue, err)
				}
			}

			// Set environment variables (these override env file values)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Make env files in a temporary directory
			tempDir := t.TempDir()

			var rcEnvs []string
			// Create env files in the temporary directory
			if tt.createRCFile {
				path := filepath.Join(tempDir, ".defang")
				if tt.rcStack.stackname != "" {
					os.Mkdir(path, 0700)
					path = filepath.Join(path, tt.rcStack.stackname)
				}

				f, err := os.Create(path)
				if err != nil {
					t.Fatalf("failed to create file %s: %v", path, err)
				}

				// Write as environment file format
				for key, value := range tt.rcStack.entries {
					if _, err := f.WriteString(key + "=" + value + "\n"); err != nil {
						t.Fatalf("failed to write to file %s: %v", path, err)
					}
					rcEnvs = append(rcEnvs, key)
				}
				f.Close()
			}

			t.Cleanup(func() {
				// Unseting env vars set for this test is handled by t.Setenv automatically
				// t.tempDir() will clean up created files

				// Unset all env vars created by loadDotDefang since it uses os.Setenv
				for _, rcEnv := range rcEnvs {
					os.Unsetenv(rcEnv)
				}
			})

			t.Chdir(tempDir)

			// simulates the actual loading sequence
			err := testConfig.loadDotDefang(tt.rcStack.stackname)
			if err != nil {
				t.Fatalf("failed to load env file: %v", err)
			}

			err = testConfig.syncFlagsWithEnv(flags)
			if err != nil {
				t.Fatalf("failed to sync flags with env vars: %v", err)
			}

			// verify the final configuration matches expectations
			if testConfig.Stack.Mode.String() != tt.expected.Stack.Mode.String() {
				t.Errorf("expected Mode to be '%s', got '%s'", tt.expected.Stack.Mode.String(), testConfig.Stack.Mode.String())
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
			if testConfig.Stack.Provider != tt.expected.Stack.Provider {
				t.Errorf("expected Provider to be '%s', got '%s'", tt.expected.Stack.Provider, testConfig.Stack.Provider)
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
		})
	}
}
