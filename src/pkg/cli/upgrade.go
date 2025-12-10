package cli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/term"
)

func Upgrade(ctx context.Context) error {
	// Find path to the current executable to determine how to upgrade
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

	prefix, err := homebrewPrefix(ctx)
	if err == nil && strings.HasPrefix(ex, prefix) {
		printInstructions("brew upgrade -g defang")
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

	// Check if we're running via npx (npm_execpath is set by npx/npm/yarn/etc)
	if _, exists := os.LookupEnv("npm_execpath"); exists {
		printInstructions("npx defang@latest")
		return nil
	}

	// Check if we're running on Windows
	if runtime.GOOS == "windows" {
		if strings.Contains(strings.ToLower(ex), "winget") {
			printInstructions("winget upgrade defang")
			return nil
		}
		// Check if we're running in PowerShell (and not CMD or Git Bash)
		_, hasMSYSTEM := os.LookupEnv("MSYSTEM")           // Git Bash/MINGW/MSYS
		_, hasPrompt := os.LookupEnv("PROMPT")             // CMD
		_, hasPSModulePath := os.LookupEnv("PSModulePath") // CMD and Powershell
		if !hasMSYSTEM && !hasPrompt && hasPSModulePath {
			printInstructions(`iwr https://s.defang.io/defang_win_amd64.zip -OutFile defang.zip; Expand-Archive defang.zip . -Force`)
			return nil
		}
	}

	// Default to the shell script
	printInstructions(`eval "$(curl -fsSL s.defang.io/install)"`)

	return nil
}

func homebrewPrefix(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "brew", "config").Output()
	if err != nil {
		return "", err
	}
	// filter out the line which includes HOMEBREW_PREFIX
	const HOMEBREW_PREFIX = "HOMEBREW_PREFIX: "
	for _, line := range strings.Split(string(output), "\n") {
		// remove the prefix from the line
		if homebrewPrefix, ok := strings.CutPrefix(line, HOMEBREW_PREFIX); ok {
			return homebrewPrefix, nil
		}
	}
	return "", errors.New("HOMEBREW_PREFIX not found in brew config")
}

func printInstructions(cmd string) {
	term.Info("To upgrade defang, run the following command:")
	term.Print("\n  ", cmd, "\n\n")
}
