package cli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Debug(ctx context.Context, c client.Client, etag, folder string) error {
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
	term.Warn(resp.General)
	for _, service := range resp.Issues {
		term.Warn(service.Details)
		for _, changes := range service.CodeChanges {
			term.Info(changes.File)
			term.Println(changes.Change)
		}
	}
	for _, request := range resp.Requests {
		term.Info(request)
	}
	return nil
}
