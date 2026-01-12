package command

import (
	"os"
	"strconv"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

/*
GlobalConfig holds the global configuration options for the Defang CLI.
These options can be configured through multiple sources with the following priority order:

 1. Command-line flags (highest priority)
 2. Environment variables (DEFANG_* prefix)
 3. Configuration files in the .defang directory (lowest priority)

Configuration Flow:

  - Default values are set when initializing the global variable
  - Environment variables are synced to struct fields (syncFlagsWithEnv)
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
	Client         *client.GrpcClient
	Cluster        string
	ColorMode      ColorMode
	Debug          bool
	HasTty         bool
	HideUpdate     bool
	ModelID        string // only for debug/generate; Pro users
	NonInteractive bool
	Stack          stacks.StackParameters
	Tenant         types.TenantNameOrID // workspace
	Verbose        bool
}

func (global *GlobalConfig) Interactive() bool {
	return !global.NonInteractive
}

/*
global is the singleton instance of GlobalConfig that holds all CLI configuration.
This instance is initialized with default values and is modified throughout
the application lifecycle as configuration sources are processed (Stack files, environment
variables, and command-line flags).
*/
var global = NewGlobalConfig()

func NewGlobalConfig() *GlobalConfig {
	color := ColorAuto
	if fromEnv, ok := os.LookupEnv("DEFANG_COLOR"); ok {
		err := color.Set(fromEnv)
		if err != nil {
			term.Debugf("invalid DEFANG_COLOR value: %v", err)
		}
	}

	provider := client.ProviderAuto
	if fromEnv, ok := os.LookupEnv("DEFANG_PROVIDER"); ok {
		err := provider.Set(fromEnv)
		if err != nil {
			term.Debugf("invalid DEFANG_PROVIDER value: %v", err)
		}
	}

	mode := modes.ModeUnspecified
	if fromEnv, ok := os.LookupEnv("DEFANG_MODE"); ok {
		err := mode.Set(fromEnv)
		if err != nil {
			term.Debugf("invalid DEFANG_MODE value: %v", err)
		}
	}

	hastty := term.IsTerminal() && !pkg.GetenvBool("CI")

	tenant := types.TenantNameOrID("")
	if fromEnv, ok := os.LookupEnv("DEFANG_WORKSPACE"); ok {
		tenant = types.TenantNameOrID(fromEnv)
	} else if fromEnv, ok := os.LookupEnv("DEFANG_ORG"); ok {
		tenant = types.TenantNameOrID(fromEnv)
		term.Warn("DEFANG_ORG is deprecated; use DEFANG_WORKSPACE instead")
	}

	return &GlobalConfig{
		ColorMode:      color,
		Cluster:        pkg.Getenv("DEFANG_FABRIC", client.DefangFabric),
		Debug:          pkg.GetenvBool("DEFANG_DEBUG"),
		HasTty:         hastty,
		HideUpdate:     pkg.GetenvBool("DEFANG_HIDE_UPDATE"),
		NonInteractive: !hastty,
		Stack: stacks.StackParameters{
			Name:     pkg.Getenv("DEFANG_STACK", ""),
			Provider: provider,
			Mode:     mode,
			Region:   client.GetRegion(provider),
		},
		Verbose: pkg.GetenvBool("DEFANG_VERBOSE"),
		Tenant:  tenant,
	}
}

func (global *GlobalConfig) ToMap() map[string]string {
	m := make(map[string]string)
	m["DEFANG_CLUSTER"] = global.Cluster
	m["DEFANG_COLOR"] = global.ColorMode.String()
	m["DEFANG_DEBUG"] = strconv.FormatBool(global.Debug)
	m["DEFANG_NON_INTERACTIVE"] = strconv.FormatBool(global.NonInteractive)
	if global.Stack.Provider != client.ProviderAuto {
		m["DEFANG_PROVIDER"] = global.Stack.Provider.String()
	}
	if global.Stack.Region != "" {
		regionVarName := client.GetRegionVarName(global.Stack.Provider)
		m[regionVarName] = global.Stack.Region
	}
	if global.Stack.Mode != modes.ModeUnspecified {
		m["DEFANG_MODE"] = global.Stack.Mode.String()
	}
	m["DEFANG_VERBOSE"] = strconv.FormatBool(global.Verbose)
	return m
}
