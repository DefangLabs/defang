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
const maxFiles = 20

var (
	errFileLimitReached = errors.New("file limit reached")
	patterns            = []string{"*.js", "*.ts", "*.py", "*.go", "requirements.txt", "package.json", "go.mod"} // TODO: add patterns for other languages
)

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

func readFile(basepath, path string) *defangv1.File {
	content, err := os.ReadFile(path)
	if err != nil {
		term.Debug("failed to read file:", err)
		return nil
	}
	if path, err = filepath.Rel(basepath, path); err != nil {
		path = filepath.Base(path)
	}
	return &defangv1.File{
		Name:    path,
		Content: string(content),
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
			term.Debug("skipped for debugging:", err)
		} else {
			services[s] = svc
		}
	}
	return services
}

func findMatchingProjectFiles(project *types.Project, services []string) []*defangv1.File {
	var files []*defangv1.File

	for _, path := range project.ComposeFiles {
		if file := readFile(project.WorkingDir, path); file != nil {
			files = append(files, file)
		}
	}

	for _, service := range getServices(project, services) {
		if service.Build != nil {
			files = append(files, findMatchingFiles(project.WorkingDir, service.Build.Context, service.Build.Dockerfile)...)
		}
		// TODO: also consider other files, lke .dockerignore, .env, etc.
	}

	return files
}

func IsProjectFile(basename string) bool {
	return filepathMatchAny(patterns, basename)
}

func filepathMatchAny(patterns []string, name string) bool {
	for _, pattern := range patterns {
		matched, err := filepath.Match(pattern, name)
		if err != nil {
			term.Debug("error matching pattern:", err)
			continue
		}
		if matched {
			return true // file matched, no need to check other patterns
		}
	}
	return false
}

func findMatchingFiles(basepath, context, dockerfile string) []*defangv1.File {
	var files []*defangv1.File

	if file := readFile(basepath, filepath.Join(context, dockerfile)); file != nil {
		files = append(files, file)
	}

	err := compose.WalkContextFolder(context, dockerfile, func(path string, info os.DirEntry, slashPath string) error {
		if info.IsDir() {
			return nil // continue to next file/directory
		}

		if len(files) >= maxFiles {
			term.Debug("file limit reached, stopping search")
			return errFileLimitReached
		}

		if IsProjectFile(info.Name()) {
			if file := readFile(basepath, path); file != nil {
				files = append(files, file)
			}
		}
		return nil
	})

	if err != nil && err != errFileLimitReached {
		term.Debug("error walking the path:", err)
	}

	return files
}
