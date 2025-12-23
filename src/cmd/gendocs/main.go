package main

import (
	"os"

	"github.com/DefangLabs/defang/src/cmd/cli/command"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra/doc"
)

// runs in docs ci to generate markdown docs
func main() {
	if len(os.Args) < 2 {
		panic("Missing required argument: docs path")
	}

	docsPath := os.Args[1]

	_ = os.Mkdir(docsPath, 0755)

	command.SetupCommands("")

	err := doc.GenMarkdownTree(command.RootCmd, docsPath)
	if err != nil {
		term.Fatal(err)
	}
}
