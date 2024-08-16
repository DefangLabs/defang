package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func Update(ctx context.Context) error {
	// Find path to the current executable to determine how to update
	ex, err := os.Executable()
	if err != nil {
		return err
	}
	term.Debugf(" - Executable: %s\n", ex)

	ex, err = filepath.EvalSymlinks(ex)
	if err != nil {
		return err
	}
	term.Debugf(" - Evaluated: %s\n", ex)

	if strings.Contains("brew/", ex) {
		printInstructions("brew upgrade defang")
	}

	if strings.Contains("/nix/store/", ex) {
		// Detect whether the user has used Flakes or nix-env
		if strings.Contains("-defang-cli-", ex) {
			printInstructions("nix-env -if https://github.com/DefangLabs/defang/archive/main.tar.gz")
		} else {
			printInstructions("nix profile install github:DefangLabs/defang#defang-bin --refresh")
		}
	}

	// Check if we're running in PowerShell
	if _, exists := os.LookupEnv("PSModulePath"); exists {
		printInstructions(`pwsh -c "iwr https://s.defang.io/defang_win_amd64.zip -OutFile defang.zip"`)

		return nil
	}

	// Default to the shell script
	printInstructions(`. <(curl -Ls https://s.defang.io/install)`)

	return nil
}

func printInstructions(cmd string) {
	fmt.Println(`Run the following command to update defang:\n\n`, cmd)
}
