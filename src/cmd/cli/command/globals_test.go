package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
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
		ColorMode:  ColorAuto,
		Debug:      false,
		HasTty:     true, // set to true just for test instead of term.IsTerminal() for consistency
		HideUpdate: false,
		// Mode:           modes.ModeUnspecified,
		NonInteractive: false, // set to false just for test instead of !term.IsTerminal() for consistency
		// ProviderID:     cliClient.ProviderAuto,
		SourcePlatform: migrate.SourcePlatformUnspecified,
		Verbose:        false,
		// Stack:          stacks.StackParameters{},
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
				// Mode:    modes.ModeHighAvailability,
				Verbose: false,
				Debug:   true,
				// Stack:          "from-flags",
				Cluster: "from-flags-cluster",
				// ProviderID:     cliClient.ProviderAWS,
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
				// Mode:    modes.ModeBalanced,
				Verbose: true,
				Debug:   false,
				// Stack:          "from-env",
				Cluster: "from-env-cluster",
				// ProviderID:     cliClient.ProviderGCP,
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
				// Mode:    modes.ModeAffordable, // env file values
				Verbose: true,
				Debug:   false,
				// Stack:          "from-env",
				Cluster: "from-env-cluster",
				// ProviderID:     cliClient.ProviderDefang,
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
			// flags.StringVarP(&testConfig.Stack, "stack", "s", testConfig.Stack, "stack name (for BYOC providers)")
			flags.Var(&testConfig.ColorMode, "color", "colorize output")
			flags.StringVar(&testConfig.Cluster, "cluster", testConfig.Cluster, "Defang cluster to connect to")
			flags.StringVar(&testConfig.Org, "org", testConfig.Org, "override GitHub organization name (tenant)")
			// flags.VarP(&testConfig.ProviderID, "provider", "P", "bring-your-own-cloud provider")
			flags.BoolVarP(&testConfig.Verbose, "verbose", "v", testConfig.Verbose, "verbose logging")
			flags.BoolVar(&testConfig.Debug, "debug", testConfig.Debug, "debug logging for troubleshooting the CLI")
			flags.BoolVar(&testConfig.NonInteractive, "non-interactive", testConfig.NonInteractive, "disable interactive prompts / no TTY")
			flags.Var(&testConfig.SourcePlatform, "from", "the platform from which to migrate the project")
			// flags.VarP(&testConfig.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))

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
			// if testConfig.Mode.String() != tt.expected.Mode.String() {
			// 	t.Errorf("expected Mode to be '%s', got '%s'", tt.expected.Mode.String(), testConfig.Mode.String())
			// }
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
			// if testConfig.ProviderID != tt.expected.ProviderID {
			// 	t.Errorf("expected ProviderID to be '%s', got '%s'", tt.expected.ProviderID, testConfig.ProviderID)
			// }
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

/*
Test_checkEnvConflicts tests the checkEnvConflicts function to ensure it correctly identifies
conflicts between environment variables set in the shell and those defined in a stack file.
It verifies that warnings are issued when conflicts are detected and that no warnings are issued
when there are no conflicts.
*/
func Test_checkEnvConflicts(t *testing.T) {
	tests := []struct {
		name           string
		stackContent   string
		shellEnv       map[string]string
		expectConflict bool
	}{
		{
			name: "Conflict detected - AWS_PROFILE",
			stackContent: `AWS_REGION="us-west-2"
DEFANG_MODE="affordable"
DEFANG_PROVIDER="aws"
AWS_PROFILE="defang-lab"`,
			shellEnv: map[string]string{
				"AWS_PROFILE": "defang-sandbox",
			},
			expectConflict: true,
		},
		{
			name: "No conflict - different values in different vars",
			stackContent: `AWS_REGION="us-west-2"
DEFANG_MODE="affordable"
DEFANG_PROVIDER="aws"`,
			shellEnv: map[string]string{
				"AWS_PROFILE": "defang-sandbox",
			},
			expectConflict: false,
		},
		{
			name: "No conflict - same value",
			stackContent: `AWS_PROFILE="defang-lab"
AWS_REGION="us-west-2"`,
			shellEnv: map[string]string{
				"AWS_PROFILE": "defang-lab",
			},
			expectConflict: false,
		},
		{
			name: "Conflict detected - multiple vars",
			stackContent: `AWS_PROFILE="defang-lab"
AWS_REGION="us-east-1"`,
			shellEnv: map[string]string{
				"AWS_PROFILE": "defang-sandbox",
				"AWS_REGION":  "us-west-2",
			},
			expectConflict: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			prevTerm := term.DefaultTerm
			var stdout, stderr bytes.Buffer
			term.DefaultTerm = term.NewTerm(os.Stdin, &stdout, &stderr)
			t.Cleanup(func() {
				term.DefaultTerm = prevTerm
			})

			// Create a temporary directory and stack file
			tempDir := t.TempDir()
			t.Chdir(tempDir)
			stackName := "test"
			stackFile := filepath.Join(tempDir, stacks.Directory, stackName)

			// Create the .defang subdirectory
			err := os.MkdirAll(filepath.Join(tempDir, stacks.Directory), 0700)
			if err != nil {
				t.Fatalf("failed to create .defang directory: %v", err)
			}

			// Write the stack file
			err = os.WriteFile(stackFile, []byte(tt.stackContent), 0644)
			if err != nil {
				t.Fatalf("failed to write stack file: %v", err)
			}

			// Set shell environment variables
			for key, value := range tt.shellEnv {
				t.Setenv(key, value)
			}

			// Call checkEnvConflicts - it displays warnings but doesn't return errors
			checkEnvConflicts(stackName)

			if tt.expectConflict && !term.HadWarnings() {
				t.Errorf("Expected warning conflicts, but no warnings were generated")
			}

			if !tt.expectConflict && term.HadWarnings() {
				t.Errorf("Expected no warning conflicts, but warnings were generated: %s", stderr.String())
			}
		})
	}
}
