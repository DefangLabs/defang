package command

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/defang-io/defang/src/pkg"
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

func printDefangHint(hint, args string) {
	if pkg.GetenvBool("DEFANG_HIDE_HINTS") || !hasTty {
		return
	}

	executable := prettyExecutable("defang")

	fmt.Printf("\n%s\n", hint)
	providerFlag := rootCmd.Flag("provider")
	clusterFlag := rootCmd.Flag("cluster")
	if providerFlag.Changed {
		fmt.Printf("\n  %s --provider %s %s\n\n", executable, providerFlag.Value.String(), args)
	} else if clusterFlag.Changed {
		fmt.Printf("\n  %s --cluster %s %s\n\n", executable, clusterFlag.Value.String(), args)
	} else {
		fmt.Printf("\n  %s %s\n\n", executable, args)
	}
	if rand.Intn(10) == 0 {
		fmt.Println("To silence these hints, do: export DEFANG_HIDE_HINTS=1")
	}
}
