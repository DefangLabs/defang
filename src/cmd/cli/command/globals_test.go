package command

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/pflag"
)

func Test_readGlobals(t *testing.T) {

	t.Run("OS env beats any .defang file", func(t *testing.T) {
		t.Chdir("testdata/with-stack")
		t.Setenv("VALUE", "from OS env")
		err := loadDotDefang("test")
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
		err := loadDotDefang("test")
		if err != nil {
			t.Fatalf("%v", err)
		}
		if v := os.Getenv("VALUE"); v != "from .defang/test" {
			t.Errorf("expected VALUE to be 'from .defang/test', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run("no stackname provided", func(t *testing.T) {

		prevTerm := term.DefaultTerm
		var stdout, stderr bytes.Buffer
		term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)
		term.SetDebug(true)
		t.Cleanup(func() {
			term.DefaultTerm = prevTerm
		})

		err := loadDotDefang("")
		if err != nil {
			t.Fatalf("%v", err)
		}

		expectedDebugMsg := " - No stack name provided; continuing without loading a stack file.\n"
		if stdout.String() != expectedDebugMsg {
			t.Errorf("expected debug message %s, got %s", expectedDebugMsg, stdout.String())
		}
	})

	t.Run("incorrect stackname used if no stack", func(t *testing.T) {
		err := loadDotDefang("non-existent-stack")
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
	}

	type stack struct {
		fileName string
		entries  map[string]string
	}

	tests := []struct {
		name       string
		stack      stack
		createFile bool
		envVars    map[string]string
		flags      map[string]string
		expected   GlobalConfig
	}{
		{
			name:       "Flags override env and env file",
			createFile: true,
			stack: stack{
				fileName: "from-flags",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "false",
					"DEFANG_DEBUG":           "true",
					"DEFANG_STACK":           "from-env-file",
					"DEFANG_FABRIC":          "from-env-file-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-file-org",
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
			name:       "Env overrides env files when no flags set",
			createFile: true,
			stack: stack{
				fileName: "from-env",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "false",
					"DEFANG_DEBUG":           "true",
					"DEFANG_STACK":           "from-env-file",
					"DEFANG_FABRIC":          "from-env-file-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-file-org",
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
			name:       "env file used when no env vars or flags set",
			createFile: true,
			flags:      map[string]string{"stack": "beta"},
			stack: stack{
				fileName: "beta",
				entries: map[string]string{
					"DEFANG_MODE":            "AFFORDABLE",
					"DEFANG_VERBOSE":         "true",
					"DEFANG_DEBUG":           "false",
					"DEFANG_FABRIC":          "from-env-file-cluster",
					"DEFANG_PROVIDER":        "defang",
					"DEFANG_ORG":             "from-env-file-org",
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
				Stack:          stacks.StackParameters{Name: "beta", Provider: cliClient.ProviderDefang, Mode: modes.ModeAffordable},
				Cluster:        "from-env-file-cluster",
				Org:            "from-env-file-org",
				SourcePlatform: migrate.SourcePlatformHeroku,
				ColorMode:      ColorAlways,
				HasTty:         false, // from env
				NonInteractive: true,  // from env
				HideUpdate:     true,  // from env
			},
		},
		// Skipped test case for now; https://github.com/DefangLabs/defang/issues/1686
		// {
		// 	name:       "stack env value should be used after loading from env file",
		// 	createFile: true,
		// 	stack: stack{
		// 		fileName: "beta",
		// 		entries: map[string]string{
		// 			"DEFANG_STACK": "from-env-file", // this value should be the final output
		// 		},
		// 	},
		// 	expected: GlobalConfig{
		// 		Stack: stacks.StackParameters{Name: "from-env-file"},
		// 	},
		// },
		{
			name:       "env file with no values",
			createFile: true,
			flags: map[string]string{
				"stack": "defang",
			},
			stack: stack{
				fileName: "defang",
			},
			expected: GlobalConfig{
				ColorMode:      ColorAuto,
				HasTty:         true,
				SourcePlatform: migrate.SourcePlatformUnspecified,
				Stack: stacks.StackParameters{
					Name:     "defang",
					Provider: cliClient.ProviderAuto,
					Mode:     modes.ModeUnspecified,
				},
			},
		},
		{
			name:       "no env file, no env vars and no flags",
			createFile: false,
			expected:   defaultConfig, // should match the initialized defaults above
		},
		{
			name:       "ignore empty debug bool",
			createFile: false,
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
			if tt.createFile {
				path := filepath.Join(tempDir, ".defang")
				if tt.stack.fileName != "" {
					os.Mkdir(path, 0700)
					path = filepath.Join(path, tt.stack.fileName)
				}

				f, err := os.Create(path)
				if err != nil {
					t.Fatalf("failed to create file %s: %v", path, err)
				}

				// Write as environment file format
				for key, value := range tt.stack.entries {
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
			err := loadDotDefang(testConfig.getStackName(flags))
			if err != nil {
				t.Fatalf("failed to load env file: %v", err)
			}

			err = testConfig.syncFlagsWithEnv(flags)
			if err != nil {
				t.Fatalf("failed to sync flags with env vars: %v", err)
			}

			// verify the final configuration matches expectations
			if !reflect.DeepEqual(testConfig.Stack, tt.expected.Stack) {
				t.Errorf("expected Stack to be '%s', got '%s'", tt.expected.Stack, testConfig.Stack)
			}
			if testConfig.Verbose != tt.expected.Verbose {
				t.Errorf("expected Verbose to be %v, got %v", tt.expected.Verbose, testConfig.Verbose)
			}
			if testConfig.Debug != tt.expected.Debug {
				t.Errorf("expected Debug to be %v, got %v", tt.expected.Debug, testConfig.Debug)
			}
			if testConfig.Cluster != tt.expected.Cluster {
				t.Errorf("expected Cluster to be '%s', got '%s'", tt.expected.Cluster, testConfig.Cluster)
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
