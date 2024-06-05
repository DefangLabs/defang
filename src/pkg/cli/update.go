package cli

import (
	"context"
	"fmt"
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

	if strings.Contains("brew/", ex) {
		term.Info(" * Updating defang via Homebrew...")
		return exec.CommandContext(ctx, "brew", "upgrade", "defang").Run()
	}

	if strings.Contains("/nix/store/", ex) {
		// Detect whether the user has used Flakes or nix-env
		if strings.Contains("-defang-cli-", ex) {
			term.Info(" * Updating defang via nix-env...")
			return exec.CommandContext(ctx, "nix-env", "-if", "https://github.com/DefangLabs/defang/archive/main.tar.gz").Run()
		} else {
			term.Info(" * Updating defang via Nix Flake...")
			return exec.CommandContext(ctx, "nix", "profile", "install", "github:DefangLabs/defang#defang-bin", "--refresh").Run()
		}
	}

	// Check if we're running in PowerShell
	if _, exists := os.LookupEnv("PSModulePath"); exists {
		term.Info(" * Updating defang via PowerShell...")
		return exec.CommandContext(ctx, "pwsh", "-c", "iwr https://s.defang.io/defang_win_amd64.zip -OutFile defang.zip").Run()
	}

	// Default to the shell script
	fmt.Println(`Run the following command to update defang:

  . <(curl -Ls https://s.defang.io/install)`)
	return nil
}
