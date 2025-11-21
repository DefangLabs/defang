package command

import (
	"os"
	"strconv"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cluster"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
	"github.com/spf13/pflag"
)

var global GlobalConfig = GlobalConfig{
	// set default values
	ColorMode:      ColorAuto,
	Cluster:        cluster.DefangFabric,
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
	/*
		GlobalConfig holds the global configuration options for the CLI.
		These options can be set via command-line flags, environment variables,
		or (.defangrc) configuration files.
	*/
	Client         *cliClient.GrpcClient
	Cluster        string
	ColorMode      ColorMode
	Debug          bool
	HasTty         bool
	HideUpdate     bool
	Mode           modes.Mode
	ModelID        string // only for debug/generate; Pro users
	NonInteractive bool
	Org            string
	ProviderID     cliClient.ProviderID
	SourcePlatform migrate.SourcePlatform // only used for 'defang init' command
	Stack          string
	Verbose        bool
}

func (r *GlobalConfig) getStack(flags *pflag.FlagSet) string {
	// If flag was changed by user, use that value, else check env var
	// This is used to determine which RC file to load at startup
	if !flags.Changed("stack") {
		if fromEnv, ok := os.LookupEnv("DEFANG_STACK"); ok {
			r.Stack = fromEnv
		}
	}

	return r.Stack
}

func (r *GlobalConfig) syncNonFlagEnvVars() error {
	// Not flags but check these environment variables for values set by user
	var err error

	// Not flags but check these environment variables
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

func (r *GlobalConfig) syncFlagsWithEnv(flags *pflag.FlagSet) error {
	// If flag was changed by user, update config from flag value (flag takes priority)
	// If flag was not changed by user, set flag from config value (env/RC file values)
	var err error

	r.Stack = r.getStack(flags)

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
			r.Debug, err = strconv.ParseBool(fromEnv)
			if err != nil {
				return err
			}
		}
	}

	if !flags.Changed("mode") {
		if fromEnv, ok := os.LookupEnv("DEFANG_MODE"); ok {
			err := r.Mode.Set(fromEnv)
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
			err = r.ProviderID.Set(fromEnv)
			if err != nil {
				return err
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

func (r *GlobalConfig) loadRC(stackName string) {
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

}
