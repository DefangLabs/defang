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

func Debug(ctx context.Context, c client.Client, etag, folder string) error {
	term.Debug("Invoking AI debugger for deployment", etag)

	// FIXME: use the project information to determine which files to send
	patterns := []string{"Dockerfile", "*compose.yaml", "*compose.yml", "*.js", "*.ts", "*.py", "*.go", "requirements.txt", "package.json", "go.mod"}
	var files []*defangv1.File
	for _, pattern := range patterns {
		fullPattern := filepath.Join(folder, pattern)
		matchingFiles, err := filepath.Glob(fullPattern)
		if err != nil {
			term.Debug("failed to find matching files:", err)
			continue
		}
		for _, fullPath := range matchingFiles {
			b, err := os.ReadFile(fullPath)
			if err != nil {
				term.Debug("failed to read file:", err)
				continue
			}
			files = append(files, &defangv1.File{
				Name:    filepath.Base(fullPath),
				Content: string(b),
			})
		}
	}

	if DoDryRun {
		return ErrDryRun
	}

	resp, err := c.Debug(ctx, &defangv1.DebugRequest{
		Etag:  etag,
		Files: files,
	})
	if err != nil {
		return err
	}

	term.Println("")
	term.Println("===================")
	term.Println("Debugging Summary")
	term.Println("===================")
	term.Println(resp.General)
	term.Println("")
	term.Println("")

	counter := 1
	for _, service := range resp.Issues {
		term.Println("-------------------")
		term.Println(fmt.Sprintf("Issue #%d", counter))
		term.Println("-------------------")
		term.Println(service.Details)
		term.Println("")
		term.Println("")

		if (len(service.CodeChanges)) > 0 {
			for _, changes := range service.CodeChanges {
				term.Println(fmt.Sprintf("File %s:", changes.File))
				term.Println("-------------------")
				term.Println(changes.Change)
				term.Println("")
				term.Println("")
			}
		}
	}
	// for _, request := range resp.Requests {
	// 	term.Info(request)
	// }
	return nil
}
