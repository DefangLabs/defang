package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// Arbitrary limit on the maximum number of files to process to avoid walking the entire drive and we have limited
// context window for the LLM also.
// FIXME: Find a better way to handle files.
const maxFiles = 20

var ErrFileLimitReached = errors.New("file limit reached")

func Debug(ctx context.Context, c client.Client, etag, folder string, services []string) error {
	term.Debug("Invoking AI debugger for deployment", etag)

	// FIXME: use the project information to determine which files to send
	patterns := []string{"Dockerfile", "*compose.yaml", "*compose.yml", "*.js", "*.ts", "*.py", "*.go", "requirements.txt", "package.json", "go.mod"}

	files := findMatchingFiles(folder, patterns)

	if DoDryRun {
		return ErrDryRun
	}

	resp, err := c.Debug(ctx, &defangv1.DebugRequest{
		Etag:     etag,
		Files:    files,
		Services: services,
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

	for counter, service := range resp.Issues {
		term.Println("-------------------")
		term.Println(fmt.Sprintf("Issue #%d", counter+1))
		term.Println("-------------------")
		term.Println(service.Details)
		term.Println("")
		term.Println("")

		if (len(service.CodeChanges)) > 0 {
			for _, changes := range service.CodeChanges {
				term.Println(fmt.Sprintf("Updated %s:", changes.File))
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

func findMatchingFiles(folder string, patterns []string) []*defangv1.File {
	var files []*defangv1.File
	fileCount := 0

	err := filepath.Walk(folder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			term.Debug("error accessing path:", err)
			return nil // continue walking
		}

		if info.IsDir() {
			return nil // continue to next file/directory
		}

		if fileCount >= maxFiles {
			term.Debug("file limit reached, stopping search")
			return ErrFileLimitReached
		}

		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				term.Debug("error matching pattern:", err)
				continue
			}
			if matched {
				b, err := os.ReadFile(path)
				if err != nil {
					term.Debug("failed to read file:", err)
					continue
				}
				files = append(files, &defangv1.File{
					Name:    filepath.Base(path),
					Content: string(b),
				})
				fileCount++
				break // file matched, no need to check other patterns
			}
		}
		return nil
	})

	if err != nil && err != ErrFileLimitReached {
		term.Debug("error walking the path:", err)
	}

	return files
}
