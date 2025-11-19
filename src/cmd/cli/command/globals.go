package command

import (
	"os"
	"strconv"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

// GLOBALS
var (
	// cluster    string
	// colorMode = ColorAuto
	// doDebug    = false
	// hasTty     = term.IsTerminal()
	// hideUpdate = false
	// mode           = modes.ModeUnspecified
	modelId string // only for debug/generate; Pro users
	// nonInteractive = !hasTty
	// org            string
	// providerID     = cliClient.ProviderAuto
	// sourcePlatform = migrate.SourcePlatformUnspecified // default to auto-detecting the source platform
	// stack          = os.Getenv("DEFANG_STACK")
	// verbose = false
)

var config GlobalConfig = GlobalConfig{
	ColorMode:      ColorAuto,
	Debug:          false,
	HasTty:         term.IsTerminal(),
	HideUpdate:     false,
	Mode:           modes.ModeUnspecified,
	NonInteractive: !term.IsTerminal(),
	ProviderID:     cliClient.ProviderAuto,
	SourcePlatform: migrate.SourcePlatformUnspecified, // default to auto-detecting the source platform
	Verbose:        false,
}

type GlobalConfig struct {
	Client         *cliClient.GrpcClient
	Cluster        string
	ColorMode      ColorMode
	Debug          bool
	HasTty         bool
	HideUpdate     bool
	Mode           modes.Mode
	NonInteractive bool
	Org            string
	ProviderID     cliClient.ProviderID
	SourcePlatform migrate.SourcePlatform // only used for 'defang init' command
	Stack          string
	Verbose        bool
}

// func (r *GlobalConfig) loadEnv() {
// 	// TODO: init each property from the environment or defaults
// 	if envStack := os.Getenv("DEFANG_STACK"); envStack != "" {
// 		r.Stack = envStack
// 	}
// 	// if envVerbose := os.Getenv("DEFANG_VERBOSE"); envVerbose != "" {
// 	// 	r.Verbose = envVerbose == "true"
// 	// }
// 	// if envDebug := os.Getenv("DEFANG_DEBUG"); envDebug != "" {
// 	// 	r.Debug = envDebug == "true"
// 	// }
// 	// if envMode := os.Getenv("DEFANG_MODE"); envMode != "" {
// 	// 	// Only apply environment mode if the mode is still unspecified (no flag was set)
// 	// 	if r.Mode == modes.ModeUnspecified {
// 	// 		r.Mode, _ = modes.Parse(envMode)
// 	// 	}
// 	// }
// 	// Initialize cluster from environment variable (DEFANG_FABRIC) or leave empty for flag default
// 	// if envCluster := os.Getenv("DEFANG_FABRIC"); envCluster != "" {
// 	// 	r.Cluster = envCluster
// 	// }
// 	// Initialize provider from environment variable (DEFANG_PROVIDER) or leave empty for flag default
// 	// if envProvider := os.Getenv("DEFANG_PROVIDER"); envProvider != "" {
// 	// 	r.ProviderID.Set(envProvider) // Use Set method since ProviderID has validation
// 	// }
// 	// // Initialize org from environment variable (DEFANG_ORG) or leave empty for flag default
// 	// if envOrg := os.Getenv("DEFANG_ORG"); envOrg != "" {
// 	// 	r.Org = envOrg
// 	// }
// 	// Initialize source platform from environment variable or leave empty for default
// 	// if envSourcePlatform := os.Getenv("DEFANG_SOURCE_PLATFORM"); envSourcePlatform != "" {
// 	// 	r.SourcePlatform.Set(envSourcePlatform)
// 	// }
// 	// // Initialize color mode from environment variable or leave empty for default
// 	// if envColorMode := os.Getenv("DEFANG_COLOR"); envColorMode != "" {
// 	// 	r.ColorMode.Set(envColorMode)
// 	// }
// 	// Initialize HasTty from environment or use terminal detection
// 	// if envTty := os.Getenv("DEFANG_TTY"); envTty != "" {
// 	// 	r.HasTty = envTty == "true"
// 	// } else {
// 	// 	r.HasTty = term.IsTerminal()
// 	// }
// 	// Initialize NonInteractive from environment or derive from HasTty
// 	// if envNonInteractive := os.Getenv("DEFANG_NON_INTERACTIVE"); envNonInteractive != "" {
// 	// 	r.NonInteractive = envNonInteractive == "true"
// 	// } else {
// 	// 	r.NonInteractive = !r.HasTty
// 	// }

// 	// if flags.Changed("non-interactive") {
// 	// 	r.NonInteractive = flags.Lookup("non-interactive").Value.String() == "true"
// 	// } else {
// 	// 	// If not explicitly set, ensure it reflects HasTty state
// 	// 	if !r.NonInteractive && !r.HasTty {
// 	// 		r.NonInteractive = true
// 	// 	}
// 	// 	flags.Set("non-interactive", fmt.Sprintf("%v", r.NonInteractive))
// 	// }

// 	// Initialize HideUpdate from environment variable
// 	// if envHideUpdate := os.Getenv("DEFANG_HIDE_UPDATE"); envHideUpdate != "" {
// 	// 	r.HideUpdate = envHideUpdate == "true"
// 	// }
// }

func (r *GlobalConfig) syncFlagsWithEnv(flags *pflag.FlagSet) {
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

	if !flags.Changed("verbose") {
		if fromEnv, ok := os.LookupEnv("DEFANG_VERBOSE"); ok {
			r.Verbose, _ = strconv.ParseBool(fromEnv)
		}
	}

	if !flags.Changed("debug") {
		if fromEnv, ok := os.LookupEnv("DEFANG_DEBUG"); ok {
			r.Debug, _ = strconv.ParseBool(fromEnv)
		}
	}

	if flags.Changed("mode") {
		if fromEnv, ok := os.LookupEnv("DEFANG_MODE"); ok {
			mode, err := modes.Parse(fromEnv)
			if err != nil {
				term.Debugf("invalid DEFANG_MODE value: %v", err)
				term.Debugf("using deafult mode from flag: %s", r.Mode.String())
			}
			r.Mode = mode
		}
	}

	if !flags.Changed("cluster") {
		if fromEnv, ok := os.LookupEnv("DEFANG_FABRIC"); ok {
			r.Cluster = fromEnv
		}
	}

	if !flags.Changed("provider") {
		if fromEnv, ok := os.LookupEnv("DEFANG_PROVIDER"); ok {
			err := r.ProviderID.Set(fromEnv)
			if err != nil {
				term.Debugf("invalid DEFANG_PROVIDER value: %v", err)
				term.Debugf("resetting ProviderID to Auto")
				r.ProviderID = cliClient.ProviderAuto
			}
		}
	}

	if !flags.Changed("org") {
		if fromEnv, ok := os.LookupEnv("DEFANG_ORG"); ok {
			r.Org = fromEnv
		}
	}

	if !flags.Changed("from") {
		if fromEnv, ok := os.LookupEnv("DEFANG_SOURCE_PLATFORM"); ok {
			err := r.SourcePlatform.Set(fromEnv)
			if err != nil {
				term.Debugf("invalid DEFANG_SOURCE_PLATFORM value: %v", err)

				term.Debugf("resetting SourcePlatform to Unspecified")
				r.SourcePlatform = migrate.SourcePlatformUnspecified
			}
		}
	}

	if !flags.Changed("color") {
		if fromEnv, ok := os.LookupEnv("DEFANG_COLOR"); ok {
			err := r.ColorMode.Set(fromEnv)
			if err != nil {
				term.Debugf("invalid DEFANG_COLOR value: %v", err)
			}
		}
	}

	if !flags.Changed("non-interactive") {
		if fromEnv, ok := os.LookupEnv("DEFANG_NON_INTERACTIVE"); ok {
			r.NonInteractive, _ = strconv.ParseBool(fromEnv)
		}
	}

	// Not a flag but check environment variable for TTY setting
	if fromEnv, ok := os.LookupEnv("DEFANG_TTY"); ok {
		r.HasTty, _ = strconv.ParseBool(fromEnv)
	}

	if fromEnv, ok := os.LookupEnv("DEFANG_HIDE_UPDATE"); ok {
		r.HideUpdate, _ = strconv.ParseBool(fromEnv)
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

	r.syncFlagsWithEnv(flags)
}
