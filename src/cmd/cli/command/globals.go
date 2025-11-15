package command

import (
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
	client         *cliClient.GrpcClient
	cluster        string
	colorMode      = ColorAuto
	doDebug        = false
	hasTty         = term.IsTerminal()
	hideUpdate     = false
	mode           = modes.ModeUnspecified
	modelId        string
	nonInteractive = !hasTty
	org            string
	providerID     = cliClient.ProviderAuto
	sourcePlatform = migrate.SourcePlatformUnspecified // default to auto-detecting the source platform
	// stack          = os.Getenv("DEFANG_STACK")
	verbose = false

	config GlobalConfig
)

type GlobalConfig struct {
	Stack string
}

func (r *GlobalConfig) loadEnv() {
	// TODO: init each property from the environment or defaults
	r.Stack = os.Getenv("DEFANG_STACK")
}

func (r *GlobalConfig) loadFlags(flags *pflag.FlagSet) {
	flags.Set("stack", r.Stack)
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
