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
	stack          = os.Getenv("DEFANG_STACK")
	verbose        = false
)

// readGlobals loads configuration from .defangrc files and returns updated values.
// It takes current values as input and returns potentially updated values from the rc files.
func readGlobals(stackName string, currentStack string, currentHasTty bool, currentHideUpdate bool, currentMode modes.Mode, currentModelId string, currentProviderID cliClient.ProviderID) (
	stack string,
	hasTty bool,
	hideUpdate bool,
	mode modes.Mode,
	modelId string,
	nonInteractive bool,
	providerID cliClient.ProviderID,
) {
	// Initialize with current values
	stack = currentStack
	hasTty = currentHasTty
	hideUpdate = currentHideUpdate
	mode = currentMode
	modelId = currentModelId
	providerID = currentProviderID

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

	stack = pkg.Getenv("DEFANG_STACK", stack)
	hasTty = term.IsTerminal() && !pkg.GetenvBool("CI")
	hideUpdate = pkg.GetenvBool("DEFANG_HIDE_UPDATE")
	mode, _ = modes.Parse(pkg.Getenv("DEFANG_MODE", string(mode)))
	modelId = pkg.Getenv("DEFANG_MODEL_ID", modelId) // for Pro users only
	nonInteractive = !hasTty
	providerID = cliClient.ProviderID(pkg.Getenv("DEFANG_PROVIDER", string(providerID)))

	return stack, hasTty, hideUpdate, mode, modelId, nonInteractive, providerID
}
