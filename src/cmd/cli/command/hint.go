package command

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
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
	if pkg.GetenvBool("DEFANG_HIDE_HINTS") || !hasTty {
		return
	}

	executable := prettyExecutable("defang")

	fmt.Printf("\n%s\n", hint)
	providerFlag := RootCmd.Flag("provider")
	clusterFlag := RootCmd.Flag("cluster")
	var prefix string
	if providerFlag.Changed {
		prefix = fmt.Sprintf("%s --provider %s", executable, providerFlag.Value.String())
	} else if clusterFlag.Changed {
		prefix = fmt.Sprintf("%s --cluster %s", executable, clusterFlag.Value.String())
	} else {
		prefix = executable
	}
	for _, arg := range cmds {
		fmt.Printf("\n  %s %s\n", prefix, arg)
	}
	if rand.Intn(10) == 0 {
		fmt.Println("To silence these hints, do: export DEFANG_HIDE_HINTS=1")
	}
}
