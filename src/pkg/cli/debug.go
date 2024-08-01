package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

// Arbitrary limit on the maximum number of files to process to avoid walking the entire drive and we have limited
// context window for the LLM also.
// FIXME: Find a better way to handle files.
const maxFiles = 20

var errFileLimitReached = errors.New("file limit reached")

func Debug(ctx context.Context, c client.Client, etag string, project *types.Project, services []string) error {
	term.Debug("Invoking AI debugger for deployment", etag)

	files := findMatchingProjectFiles(project, services)

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

func readFile(path string) *defangv1.File {
	b, err := os.ReadFile(path)
	if err != nil {
		term.Debug("failed to read file:", err)
		return nil
	}
	return &defangv1.File{
		Name:    filepath.Base(path),
		Content: string(b),
	}
}

func getServices(project *types.Project, names []string) types.Services {
	// project.GetServices(…) aborts if any service is not found, so we filter them out ourselves
	if len(names) == 0 {
		return project.Services
	}
	services := types.Services{}
	for _, s := range names {
		if svc, err := project.GetService(s); err != nil {
			term.Debugf("can't get service %q", s)
		} else {
			services[s] = svc
		}
	}
	return services
}

func findMatchingProjectFiles(project *types.Project, services []string) []*defangv1.File {
	var files []*defangv1.File

	for _, path := range project.ComposeFiles {
		if file := readFile(path); file != nil {
			files = append(files, file)
		}
	}

	for _, service := range getServices(project, services) {
		if service.Build != nil {
			files = append(files, findMatchingFiles(service.Build.Context, service.Build.Dockerfile)...)
		}
		// TODO: also consider other files, lke .dockerignore, .env, etc.
	}

	return files
}

func findMatchingFiles(folder, dockerfile string) []*defangv1.File {
	var files []*defangv1.File
	patterns := []string{"*.js", "*.ts", "*.py", "*.go", "requirements.txt", "package.json", "go.mod"}

	if file := readFile(filepath.Join(folder, dockerfile)); file != nil {
		files = append(files, file)
	}

	err := compose.WalkContextFolder(folder, dockerfile, func(path string, info os.DirEntry, slashPath string) error {
		if info.IsDir() {
			return nil // continue to next file/directory
		}

		if len(files) >= maxFiles {
			term.Debug("file limit reached, stopping search")
			return errFileLimitReached
		}

		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				term.Debug("error matching pattern:", err)
				continue
			}
			if matched {
				if file := readFile(path); file != nil {
					files = append(files, file)
					break // file matched, no need to check other patterns
				}
			}
		}
		return nil
	})

	if err != nil && err != errFileLimitReached {
		term.Debug("error walking the path:", err)
	}

	return files
}
