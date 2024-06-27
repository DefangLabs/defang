package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func GenerateWithAI(ctx context.Context, client client.Client, language, dir, description string) ([]string, error) {
	if DoDryRun {
		term.Warn("Dry run, not generating files")
		return nil, ErrDryRun
	}

	response, err := client.GenerateFiles(ctx, &defangv1.GenerateFilesRequest{
		AgreeTos: true, // agreement was already checked by the caller
		Language: language,
		Prompt:   description,
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
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	for _, file := range response.Files {
		// Print the files that were generated
		fmt.Println("   -", file.Name)
		// TODO: this will overwrite existing files
		if err = os.WriteFile(filepath.Join(dir, file.Name), []byte(file.Content), 0644); err != nil {
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
