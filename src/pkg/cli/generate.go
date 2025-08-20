package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

var SupportedLanguages = []string{"Nodejs", "Golang", "Python"}

type GenerateArgs struct {
	Description string
	Folder      string
	Language    string
	ModelId     string
}

func GenerateWithAI(ctx context.Context, client client.FabricClient, args GenerateArgs) ([]string, error) {
	if dryrun.DoDryRun {
		term.Warn("Dry run, no project files will be generated")
		return nil, dryrun.ErrDryRun
	}

	response, err := client.GenerateFiles(ctx, &defangv1.GenerateFilesRequest{
		AgreeTos: true, // agreement was already checked by the caller
		Language: args.Language,
		ModelId:  args.ModelId,
		Prompt:   args.Description,
	})
	if err != nil {
		return nil, err
	}

	if term.DoDebug() {
		// Print the files that were generated
		for _, file := range response.Files {
			term.Printc(term.DebugColor, file.Name+"\n```")
			term.Printc(term.DebugColor, file.Content)
			term.Printc(term.DebugColor, "```")
			term.Println("")
			term.Println("")
		}
	}

	// Write each file to disk
	term.Info("Writing files to disk...")
	if err := os.MkdirAll(args.Folder, 0755); err != nil {
		return nil, err
	}
	for _, file := range response.Files {
		// Print the files that were generated
		fmt.Println("   -", file.Name)
		// TODO: this will overwrite existing files
		if err = os.WriteFile(filepath.Join(args.Folder, file.Name), []byte(file.Content), 0644); err != nil {
			return nil, err
		}
	}

	// put the file names in an array
	var fileNames []string
	for _, file := range response.Files {
		fileNames = append(fileNames, file.Name)
	}

	return fileNames, nil
}
