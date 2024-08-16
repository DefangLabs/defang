package cli

import (
	"context"
	"os"
	"os/exec"
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

	if strings.HasPrefix(ex, homebrewPrefix(ctx)) {
		printInstructions("brew upgrade defang")
		return nil
	}

	if strings.HasPrefix(ex, "/nix/store/") {
		// Detect whether the user has used Flakes or nix-env
		if strings.Contains("-defang-cli-", ex) {
			printInstructions("nix-env -if https://github.com/DefangLabs/defang/archive/main.tar.gz")
		} else {
			printInstructions("nix profile install github:DefangLabs/defang#defang-bin --refresh")
		}
		return nil
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

func homebrewPrefix(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "brew", "config").Output()
	if err != nil {
		return ""
	}
	homebrewPrefix := ""
	// filter out the line which includes HOMEBREW_PREFIX
	for _, line := range strings.Split(string(output), "\n") {
		config_key := "HOMEBREW_PREFIX: "
		if strings.HasPrefix(line, config_key) {
			// remove the prefix from the line
			homebrewPrefix = strings.TrimPrefix(line, config_key)
			break
		}
	}
	return homebrewPrefix
}

func printInstructions(cmd string) {
	term.Info("To update defang, run the following command:\n\n", cmd)
}
