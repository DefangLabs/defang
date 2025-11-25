package command

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func prettyExecutable(def string) string {
	if os.Args[0] == def {
		return def
	}
	executable, _ := os.Executable()
	if executable == "" || strings.HasPrefix(executable, os.TempDir()) {
		// If the binary is from the temp folder, default to def
		return def
	}
	wd, err := os.Getwd()
	if err != nil {
		return def
	}

	// for npm/npx defang is executed within a child process,
	// but we want to use parent process command line
	execLine := os.Getenv("DEFANG_COMMAND_EXECUTOR")
	if execLine != "" {
		return execLine
	}

	executable, _ = filepath.Rel(wd, executable)
	if executable == def {
		executable = "./" + def // to ensure it's executable
	}

	if executable == "" {
		return def
	}
	return executable
}

func printDefangHint(hint string, cmds ...string) {
	if pkg.GetenvBool("DEFANG_HIDE_HINTS") || !global.HasTty {
		return
	}

	executable := prettyExecutable("defang")

	term.Printf("\n%s\n\n", hint)
	if providerFlag := RootCmd.Flag("provider"); providerFlag.Changed {
		executable += " --provider=" + providerFlag.Value.String()
	}
	if clusterFlag := RootCmd.Flag("cluster"); clusterFlag.Changed {
		executable += " --cluster=" + clusterFlag.Value.String()
	}
	if orgFlag := RootCmd.Flag("org"); orgFlag.Changed {
		executable += " --org=" + orgFlag.Value.String()
	}
	for _, arg := range cmds {
		term.Printf("  %s %s\n\n", executable, arg)
	}
	if pkg.RandomIndex(10) == 0 {
		term.Println("To silence these hints, do: export DEFANG_HIDE_HINTS=1")
	}
}
