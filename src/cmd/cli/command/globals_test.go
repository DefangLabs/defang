package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
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
		err := loadStackFile("test")
		if err != nil {
			t.Fatalf("%v", err)
		}
		if v := os.Getenv("VALUE"); v != "from OS env" {
			t.Errorf("expected VALUE to be 'from OS env', got '%s'", v)
		}
		os.Unsetenv("VALUE")
	})

	t.Run("incorrect stackname used if no stack", func(t *testing.T) {
		err := loadStackFile("non-existent-stack")
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
		Stack:          stacks.StackParameters{Provider: client.ProviderAuto, Mode: modes.ModeUnspecified},
		Cluster:        "",
		Tenant:         "",
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
				"from":            "heroku",
				"color":           "always",
				"non-interactive": "false",
			},
			expected: GlobalConfig{
				Verbose: false,
				Debug:   true,
				Stack: stacks.StackParameters{
					Name:     "from-flags",
					Provider: client.ProviderAWS,
					Mode:     modes.ModeHighAvailability,
				},
				Cluster:        "from-flags-cluster",
				Tenant:         "",
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
				"DEFANG_SOURCE_PLATFORM": "heroku",
				"DEFANG_COLOR":           "auto",
				"DEFANG_TTY":             "true",
				"DEFANG_NON_INTERACTIVE": "false",
				"DEFANG_HIDE_UPDATE":     "false",
			},
			expected: GlobalConfig{
				Verbose: true,
				Debug:   false,
				Stack: stacks.StackParameters{
					Name:     "from-env",
					Provider: client.ProviderGCP,
					Mode:     modes.ModeBalanced,
				},
				Cluster:        "from-env-cluster",
				Tenant:         "",
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
					"DEFANG_SOURCE_PLATFORM": "heroku",
					"DEFANG_COLOR":           "always",
					"DEFANG_TTY":             "false",
					"DEFANG_NON_INTERACTIVE": "true",
					"DEFANG_HIDE_UPDATE":     "true",
				},
			},
			expected: GlobalConfig{
				Verbose: true,
				Debug:   false,
				Stack: stacks.StackParameters{
					Name:     "from-env",
					Provider: client.ProviderDefang,
					Mode:     modes.ModeAffordable,
				},
				Cluster:        "from-env-cluster",
				Tenant:         "",
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
			flags.StringVarP(&testConfig.Stack.Name, "stack", "s", testConfig.Stack.Name, "stack name (for BYOC providers)")
			flags.Var(&testConfig.ColorMode, "color", "colorize output")
			flags.StringVar(&testConfig.Cluster, "cluster", testConfig.Cluster, "Defang cluster to connect to")
			flags.StringVar(&testConfig.Tenant, "workspace", testConfig.Tenant, "workspace name (tenant)")
			flags.VarP(&testConfig.Stack.Provider, "provider", "P", "bring-your-own-cloud provider")
			flags.BoolVarP(&testConfig.Verbose, "verbose", "v", testConfig.Verbose, "verbose logging")
			flags.BoolVar(&testConfig.Debug, "debug", testConfig.Debug, "debug logging for troubleshooting the CLI")
			flags.BoolVar(&testConfig.NonInteractive, "non-interactive", testConfig.NonInteractive, "disable interactive prompts / no TTY")
			flags.Var(&testConfig.SourcePlatform, "from", "the platform from which to migrate the project")
			flags.VarP(&testConfig.Stack.Mode, "mode", "m", "deployment mode")

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
			err := loadStackFile(tt.rcStack.stackname)
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

func TestTenantFlagWinsOverEnv(t *testing.T) {
	cfg := GlobalConfig{
		Cluster: client.DefangFabric,
	}
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.StringVar(&cfg.Tenant, "workspace", cfg.Tenant, "workspace name")
	flags.StringVar(&cfg.Cluster, "cluster", cfg.Cluster, "cluster")

	if err := flags.Set("workspace", "flag-workspace"); err != nil {
		t.Fatalf("failed to set workspace flag: %v", err)
	}
	t.Setenv("DEFANG_WORKSPACE", "env-workspace")

	if err := cfg.syncFlagsWithEnv(flags); err != nil {
		t.Fatalf("failed to sync flags with env vars: %v", err)
	}

	if cfg.Tenant != "flag-workspace" {
		t.Fatalf("expected tenant from flag, got %q", cfg.Tenant)
	}
}

func TestTenantEnvSources(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name: "workspace env wins",
			envVars: map[string]string{
				"DEFANG_WORKSPACE": "workspace-env",
				"DEFANG_TENANT":    "tenant-env",
				"DEFANG_ORG":       "org-env",
			},
			expected: "workspace-env",
		},
		{
			name: "tenant env ignored",
			envVars: map[string]string{
				"DEFANG_TENANT": "tenant-env",
			},
			expected: "",
		},
		{
			name: "org env fallback",
			envVars: map[string]string{
				"DEFANG_ORG": "org-env",
			},
			expected: "org-env",
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

			// Create the .defang subdirectory
			err := os.MkdirAll(filepath.Join(tempDir, stacks.Directory), 0700)
			if err != nil {
				t.Fatalf("failed to create .defang directory: %v", err)
			}
			cfg := GlobalConfig{
				Cluster: client.DefangFabric,
			}
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.StringVar(&cfg.Tenant, "workspace", cfg.Tenant, "workspace name")
			flags.StringVar(&cfg.Cluster, "cluster", cfg.Cluster, "cluster")

			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			if err := cfg.syncFlagsWithEnv(flags); err != nil {
				t.Fatalf("failed to sync flags with env vars: %v", err)
			}

			if cfg.Tenant != tt.expected {
				t.Fatalf("expected tenant %q, got %q", tt.expected, cfg.Tenant)
			}
		})
	}
}
