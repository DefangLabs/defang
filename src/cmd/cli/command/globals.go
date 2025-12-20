package command

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

/*
GlobalConfig holds the global configuration options for the Defang CLI.
These options can be configured through multiple sources with the following priority order:

 1. Command-line flags (highest priority)
 2. Environment variables (DEFANG_* prefix)
 3. Configuration files in the .defang directory (lowest priority)

Configuration Flow:

  - Default values are set when initializing the global variable
  - Stack files are loaded to set environment variables (loadStackFile)
  - Environment variables and Stack file values are synced to struct fields (syncFlagsWithEnv)
  - Command-line flags take precedence over all other sources

Adding New Configuration Options:
To add a new configuration option, you must update these components:

1. Add the field to this GlobalConfig struct with appropriate type and Go documentation

2. Set a default value in the global variable initialization (top of this file)

3. Register the command-line flag in SetupCommands() function (commands.go):

  - For boolean flags: use BoolVar() or BoolVarP()
  - For string flags: use StringVar() or StringVarP()
  - For custom types: use Var() or VarP() (type must implement pflag.Value interface)
  - Example: RootCmd.PersistentFlags().BoolVar(&global.NewFlag, "new-flag", global.NewFlag, "description")

4. Add environment variable synchronization in syncFlagsWithEnv() method:

  - Check if flag was changed by user with flags.Changed("flag-name")
  - If not changed, read from environment variable DEFANG_FLAG_NAME
  - Handle type conversion (strconv.ParseBool for bool, direct assignment for string, etc.)

Example pattern:

	if !flags.Changed("new-flag") {
		if fromEnv, ok := os.LookupEnv("DEFANG_NEW_FLAG"); ok {
			global.NewFlag, err = strconv.ParseBool(fromEnv) // for bool
			if err != nil {
				return err
			}
		}
	}

5. For non-flag environment variables (like HasTty, HideUpdate), add handling in syncNonFlagEnvVars()

Note: Ensure the flag name, environment variable name, and struct field name are consistent
and follow the established naming conventions.
*/
type GlobalConfig struct {
	Client         *cliClient.GrpcClient
	Cluster        string
	ColorMode      ColorMode
	Debug          bool
	HasTty         bool
	HideUpdate     bool
	ModelID        string // only for debug/generate; Pro users
	NonInteractive bool
	Tenant         string
	SourcePlatform migrate.SourcePlatform // only used for 'defang init' command
	Stack          stacks.StackParameters
	Verbose        bool
}

/*
global is the singleton instance of GlobalConfig that holds all CLI configuration.
This instance is initialized with default values and is modified throughout
the application lifecycle as configuration sources are processed (Stack files, environment
variables, and command-line flags).
*/
var global GlobalConfig = GlobalConfig{
	ColorMode:      ColorAuto,
	Cluster:        cluster.DefangFabric,
	Debug:          pkg.GetenvBool("DEFANG_DEBUG"),
	HasTty:         term.IsTerminal(),
	HideUpdate:     false,
	NonInteractive: !term.IsTerminal(),
	Stack:          stacks.StackParameters{Provider: cliClient.ProviderAuto, Mode: modes.ModeUnspecified},
	SourcePlatform: migrate.SourcePlatformUnspecified, // default to auto-detecting the source platform
	Verbose:        false,
}

/*
getStackName determines the stack name to use
The returned stack name is used to determine which stack-specific file
should be loaded during configuration initialization.
*/
func (r *GlobalConfig) getStackName(flags *pflag.FlagSet) string {
	if !flags.Changed("stack") {
		if fromEnv, ok := os.LookupEnv("DEFANG_STACK"); ok {
			r.Stack.Name = fromEnv
		}
	}

	return r.Stack.Name
}

/*
syncNonFlagEnvVars handles environment variables that are not associated with command-line flags.
This ensures that these settings can still be configured via environment variables even though
they don't have corresponding CLI flags (e.g., HasTty, HideUpdate).
*/
func (r *GlobalConfig) syncNonFlagEnvVars() error {
	var err error

	// Check these environment variables that don't have corresponding command-line flags
	if fromEnv, ok := os.LookupEnv("DEFANG_TTY"); ok {
		r.HasTty, err = strconv.ParseBool(fromEnv)
		if err != nil {
			return err
		}
	}

	if fromEnv, ok := os.LookupEnv("DEFANG_HIDE_UPDATE"); ok {
		r.HideUpdate, err = strconv.ParseBool(fromEnv)
		if err != nil {
			return err
		}
	}

	return nil
}

/*
syncFlagsWithEnv synchronizes configuration values from environment variables into the GlobalConfig struct.
This function implements the priority system where command-line flags take precedence over environment variables.

Logic for each configuration option:

  - If the flag was explicitly set by the user (flags.Changed), use the flag value (already set by cobra)
  - If the flag was NOT set by the user, check for the corresponding DEFANG_* environment variable
  - If the environment variable exists, parse it and update the struct field
  - Environment variables can come from the shell environment or stack files loaded by loadStackFile()

This ensures the priority order: command-line flags > environment variables > stack file values > defaults
*/
func (r *GlobalConfig) syncFlagsWithEnv(flags *pflag.FlagSet) error {
	var err error

	// called once more in case stack name was changed by an RC file
	r.Stack.Name = r.getStackName(flags)

	if !flags.Changed("verbose") {
		if fromEnv, ok := os.LookupEnv("DEFANG_VERBOSE"); ok {
			r.Verbose, err = strconv.ParseBool(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	if !flags.Changed("debug") {
		if fromEnv, ok := os.LookupEnv("DEFANG_DEBUG"); ok {
			// Ignore error: our action sets this to empty value; default to false if parsing fails
			r.Debug, _ = strconv.ParseBool(fromEnv)
		}
	}

	if !flags.Changed("mode") {
		if fromEnv, ok := os.LookupEnv("DEFANG_MODE"); ok {
			err := r.Stack.Mode.Set(fromEnv)
			if err != nil {
				term.Debugf("invalid DEFANG_MODE value: %v", err)
			}
		}
	}

	if !flags.Changed("cluster") {
		if fromEnv, ok := os.LookupEnv("DEFANG_FABRIC"); ok {
			r.Cluster = fromEnv
		}
	}

	if !flags.Changed("provider") {
		if fromEnv, ok := os.LookupEnv("DEFANG_PROVIDER"); ok {
			err = r.Stack.Provider.Set(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	if !flags.Changed("workspace") {
		if fromEnv, ok := os.LookupEnv("DEFANG_WORKSPACE"); ok {
			r.Tenant = fromEnv
		} else if fromEnv, ok := os.LookupEnv("DEFANG_ORG"); ok {
			r.Tenant = fromEnv
			term.Warn("DEFANG_ORG is deprecated; use DEFANG_WORKSPACE instead")
		}
	}

	if !flags.Changed("from") {
		if fromEnv, ok := os.LookupEnv("DEFANG_SOURCE_PLATFORM"); ok {
			err = r.SourcePlatform.Set(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	if !flags.Changed("color") {
		if fromEnv, ok := os.LookupEnv("DEFANG_COLOR"); ok {
			err = r.ColorMode.Set(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	if !flags.Changed("non-interactive") {
		if fromEnv, ok := os.LookupEnv("DEFANG_NON_INTERACTIVE"); ok {
			r.NonInteractive, err = strconv.ParseBool(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	return r.syncNonFlagEnvVars()
}

/*
loadStackFile loads configuration values from stack files into environment variables.

If stackName is provided, loads .defang.<stackName> (required - returns error if missing/invalid)

Important: stack files have the lowest priority in the configuration hierarchy.
They will NOT override environment variables that are already set, since
godotenv.Load respects existing environment variables. Stack-specific stack files
are considered required when specified.

This function also checks for conflicts between environment variables in the stack file
and existing shell environment variables, and warns the user if any are found.
*/
func loadStackFile(stackName string) error {
	if stackName != "" {
		// Check for conflicts before loading
		err := checkEnvConflicts(stackName)
		if err != nil {
			return err
		}

		return stacks.Load(stackName) // ensure stack exists
	}

	return nil
}

/*
checkEnvConflicts reads the stack file and checks if any environment variables
in the file conflict with existing shell environment variables. If conflicts are
found, it warns the user that the shell environment variable will take precedence.
*/
func checkEnvConflicts(stackName string) error {
	path, err := filepath.Abs(filepath.Join(stacks.Directory, stackName))
	if err != nil {
		return err
	}

	// Read the stack file
	stackEnv, err := godotenv.Read(path)
	if err != nil {
		// If we can't read the file, the subsequent stacks.Load which calls godotenv.Load will not work either
		return err
	}

	// Check for conflicts with existing shell environment
	var conflicts []string
	for key, stackValue := range stackEnv {
		if shellValue, ok := os.LookupEnv(key); ok && shellValue != stackValue {
			conflicts = append(conflicts, key)
		}
	}

	// Warn about conflicts
	if len(conflicts) > 0 {
		// Sort conflicts for deterministic output
		sort.Strings(conflicts)

		term.Warnf("Some variables from the stack file %q are overridden by your shell environment.", stackName)
		for _, key := range conflicts {
			term.Printf("  %s=%q\n", key, os.Getenv(key))
		}
		term.Println("Unset these variables in your shell to use stack values instead.")
	}
	return nil
}
