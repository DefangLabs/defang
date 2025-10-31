package command

import (
	"os"

	"github.com/DefangLabs/defang/src/pkg"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/migrate"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/joho/godotenv"
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
	stack          string
	verbose        = false
)

func readGlobals(stackFile string) {
	if stackFile != "" {
		rcfile := ".defangrc." + stackFile
		if err := godotenv.Load(rcfile); err != nil {
			term.Debugf("could not load %s: %v", rcfile, err)
		} else {
			term.Debugf("loaded globals from %s", rcfile)
		}
	}
	if err := godotenv.Load(".defangrc"); err != nil {
		term.Debugf("could not load %s: %v", ".defangrc", err)
	} else {
		term.Debugf("loaded globals from %s", ".defangrc")
	}

	stack = os.Getenv("DEFANG_STACK")
	hasTty = term.IsTerminal() && !pkg.GetenvBool("CI")
	hideUpdate = pkg.GetenvBool("DEFANG_HIDE_UPDATE")
	mode, _ = modes.Parse(os.Getenv("DEFANG_MODE"))
	modelId = os.Getenv("DEFANG_MODEL_ID") // for Pro users only
	nonInteractive = !hasTty
	providerID = cliClient.ProviderID(pkg.Getenv("DEFANG_PROVIDER", "auto"))
}
