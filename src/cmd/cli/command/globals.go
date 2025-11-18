package command

import (
	"fmt"
	"os"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

// GLOBALS
var (
	client *cliClient.GrpcClient
	// cluster    string
	colorMode = ColorAuto
	// doDebug    = false
	hasTty     = term.IsTerminal()
	hideUpdate = false
	// mode           = modes.ModeUnspecified
	modelId        string
	nonInteractive = !hasTty
	// org            string
	// providerID     = cliClient.ProviderAuto
	// sourcePlatform = migrate.SourcePlatformUnspecified // default to auto-detecting the source platform
	// stack          = os.Getenv("DEFANG_STACK")
	// verbose = false

	config GlobalConfig
)

type GlobalConfig struct {
	Stack          string
	Verbose        bool
	Debug          bool
	Mode           modes.Mode
	Cluster        string
	ProviderID     cliClient.ProviderID
	Org            string
	SourcePlatform migrate.SourcePlatform
}

func (r *GlobalConfig) loadEnv() {
	// TODO: init each property from the environment or defaults
	if envStack := os.Getenv("DEFANG_STACK"); envStack != "" {
		r.Stack = envStack
	}
	if envVerbose := os.Getenv("DEFANG_VERBOSE"); envVerbose != "" {
		r.Verbose = envVerbose == "true"
	}
	if envDebug := os.Getenv("DEFANG_DEBUG"); envDebug != "" {
		r.Debug = envDebug == "true"
	}
	if envMode := os.Getenv("DEFANG_MODE"); envMode != "" {
		// Only apply environment mode if the mode is still unspecified (no flag was set)
		if r.Mode == modes.ModeUnspecified {
			r.Mode, _ = modes.Parse(envMode)
		}
	}
	// Initialize cluster from environment variable (DEFANG_FABRIC) or leave empty for flag default
	if envCluster := os.Getenv("DEFANG_FABRIC"); envCluster != "" {
		r.Cluster = envCluster
	}
	// Initialize provider from environment variable (DEFANG_PROVIDER) or leave empty for flag default
	if envProvider := os.Getenv("DEFANG_PROVIDER"); envProvider != "" {
		r.ProviderID.Set(envProvider) // Use Set method since ProviderID has validation
	}
	// Initialize org from environment variable (DEFANG_ORG) or leave empty for flag default
	if envOrg := os.Getenv("DEFANG_ORG"); envOrg != "" {
		r.Org = envOrg
	}
	// Initialize source platform from environment variable or leave empty for default
	if envSourcePlatform := os.Getenv("DEFANG_SOURCE_PLATFORM"); envSourcePlatform != "" {
		r.SourcePlatform.Set(envSourcePlatform)
	}
}

func (r *GlobalConfig) loadFlags(flags *pflag.FlagSet) {
	if flags == nil {
		return
	}
	// If flag was changed by user, update config from flag value (flag takes priority)
	// If flag was not changed by user, set flag from config value (env/RC file values)

	if flags.Changed("stack") {
		r.Stack = flags.Lookup("stack").Value.String()
	} else {
		flags.Set("stack", r.Stack)
	}

	if flags.Changed("verbose") {
		r.Verbose = flags.Lookup("verbose").Value.String() == "true"
	} else {
		flags.Set("verbose", fmt.Sprintf("%v", r.Verbose))
	}

	if flags.Changed("debug") {
		r.Debug = flags.Lookup("debug").Value.String() == "true"
	} else {
		flags.Set("debug", fmt.Sprintf("%v", r.Debug))
	}

	if flags.Changed("mode") {
		r.Mode, _ = modes.Parse(flags.Lookup("mode").Value.String())
	} else {
		flags.Set("mode", r.Mode.String())
	}

	if flags.Changed("cluster") {
		r.Cluster = flags.Lookup("cluster").Value.String()
	} else {
		// If config has no value, use flag's default value
		if r.Cluster == "" {
			r.Cluster = flags.Lookup("cluster").DefValue
		}
		flags.Set("cluster", r.Cluster)
	}

	if flags.Changed("provider") {
		r.ProviderID.Set(flags.Lookup("provider").Value.String())
	} else {
		// If config has no value, use flag's default value
		if r.ProviderID.String() == "" {
			r.ProviderID.Set(flags.Lookup("provider").DefValue)
		}
		flags.Set("provider", r.ProviderID.String())
	}

	if flags.Changed("org") {
		r.Org = flags.Lookup("org").Value.String()
	} else {
		// If config has no value, use flag's default value
		if r.Org == "" {
			r.Org = flags.Lookup("org").DefValue
		}
		flags.Set("org", r.Org)
	}

	if flags.Changed("from") {
		r.SourcePlatform.Set(flags.Lookup("from").Value.String())
	} else {
		// If config has no value, use default (unspecified)
		if r.SourcePlatform.String() == "" {
			r.SourcePlatform = migrate.SourcePlatformUnspecified
		}
		// Note: 'from' flag is only on initCmd, not global, so we don't set it here
	}
}

func (r *GlobalConfig) loadRC(stackName string, flags *pflag.FlagSet) {
	if stackName != "" {
		rcfile := ".defangrc." + stackName
		if err := godotenv.Load(rcfile); err != nil {
			term.Debugf("could not load %s: %v", rcfile, err)
		} else {
			term.Debugf("loaded globals from %s", rcfile)
		}
	}
	const rcfile = ".defangrc"
	if err := godotenv.Load(rcfile); err != nil {
		term.Debugf("could not load %s: %v", rcfile, err)
	} else {
		term.Debugf("loaded globals from %s", rcfile)
	}

	r.loadEnv()
	r.loadFlags(flags)
	term.Debug("loadRC finished: config.Cluster =", r.Cluster)
}

// func readGlobals(stackName string) GlobalConfig {

// 	stack := pkg.Getenv("DEFANG_STACK", stackName)
// 	hasTty = term.IsTerminal() && !pkg.GetenvBool("CI")
// 	hideUpdate = pkg.GetenvBool("DEFANG_HIDE_UPDATE")
// 	mode, _ = modes.Parse(pkg.Getenv("DEFANG_MODE", mode.String()))
// 	modelId = pkg.Getenv("DEFANG_MODEL_ID", modelId) // for Pro users only
// 	nonInteractive = !hasTty
// 	providerID = cliClient.ProviderID(pkg.Getenv("DEFANG_PROVIDER", providerID.String()))

// 	return GlobalConfig{
// 		Stack: stack,
// 	}
// }
