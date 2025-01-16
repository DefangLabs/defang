package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

// Arbitrary limit on the maximum number of files to process to avoid walking the entire drive and we have limited
// context window for the LLM also.
const maxFiles = 20

var (
	ErrDebugSkipped = errors.New("debug skipped")

	errFileLimitReached = errors.New("file limit reached")
	patterns            = []string{"*.js", "*.ts", "*.py", "*.go", "requirements.txt", "package.json", "go.mod"} // TODO: add patterns for other languages
)

func InteractiveDebugDeployment(ctx context.Context, c client.FabricClient, project *compose.Project, p client.Provider, etag types.ETag, failedServices []string) error {
	return interactiveDebug(ctx, c, p, etag, project, failedServices, nil)
}

func InteractiveDebugForLoadError(ctx context.Context, c client.FabricClient, project *compose.Project, loadError error) error {
	return interactiveDebug(ctx, c, nil, "", project, nil, loadError)
}

func interactiveDebug(ctx context.Context, c client.FabricClient, p client.Provider, etag types.ETag, project *compose.Project, failedServices []string, loadError error) error {
	var aiDebug bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Would you like to debug the deployment with AI?",
		Help:    "This will send logs and artifacts to our backend and attempt to diagnose the issue and provide a solution.",
	}, &aiDebug, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		track.Evt("Debug Prompt Failed", P("etag", etag), P("reason", err), P("loadErr", loadError))
		return err
	} else if !aiDebug {
		track.Evt("Debug Prompt Skipped", P("etag", etag), P("loadErr", loadError))
		return ErrDebugSkipped
	}

	track.Evt("Debug Prompt Accepted", P("etag", etag), P("loadErr", loadError))

	if loadError != nil {
		if err := debugComposeFileLoadError(ctx, c, project, loadError); err != nil {
			term.Warnf("Failed to debug compose file load: %v", err)
			return err
		}
	} else if etag != "" {
		if err := DebugDeployment(ctx, c, p, etag, project, failedServices); err != nil {
			term.Warnf("Failed to debug deployment: %v", err)
			return err
		}
	} else {
		return errors.New("no information to use for debugger")
	}

	var goodBad bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Was the debugging helpful?",
		Help:    "Please provide feedback to help us improve the debugging experience.",
	}, &goodBad); err != nil {
		track.Evt("Debug Feedback Prompt Failed", P("etag", etag), P("reason", err), P("loadErr", loadError))
	} else {
		track.Evt("Debug Feedback Prompt Answered", P("etag", etag), P("feedback", goodBad), P("loadErr", loadError))
	}
	return nil
}

func DebugDeployment(ctx context.Context, c client.FabricClient, p client.Provider, etag types.ETag, project *compose.Project, failedServices []string) error {
	term.Debugf("Invoking AI debugger for deployment %q", etag)

	files := findMatchingProjectFiles(project, failedServices)

	if DoDryRun {
		return ErrDryRun
	}

	req := defangv1.DebugRequest{
		Etag:     etag,
		Files:    files,
		Services: failedServices,
		Project:  project.Name,
	}

	err := p.Query(ctx, &req)
	if err != nil {
		return err
	}

	resp, err := c.Debug(ctx, &req)
	if err != nil {
		return err
	}

	printDebugReport(resp)

	// for _, request := range resp.Requests {
	// 	term.Info(request)
	// }
	return nil
}

func debugComposeFileLoadError(ctx context.Context, c client.FabricClient, project *compose.Project, loadErr error) error {
	term.Debugf("Invoking AI debugger for load error: %v", loadErr)

	files := findMatchingProjectFiles(project, nil)

	if DoDryRun {
		return ErrDryRun
	}

	req := defangv1.DebugRequest{
		Files:   files,
		Project: project.Name,
		Logs:    loadErr.Error(),
	}

	resp, err := c.Debug(ctx, &req)
	if err != nil {
		return err
	}

	printDebugReport(resp)
	return nil
}

func printDebugReport(resp *defangv1.DebugResponse) {
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

func getServices(project *compose.Project, names []string) compose.Services {
	// project.GetServices(…) aborts if any service is not found, so we filter them out ourselves
	if len(names) == 0 {
		return project.Services
	}
	services := compose.Services{}
	for _, s := range names {
		if svc, err := project.GetService(s); err != nil {
			term.Debug("skipped for debugging:", err)
		} else {
			services[s] = svc
		}
	}
	return services
}

func findMatchingProjectFiles(project *compose.Project, services []string) []*defangv1.File {
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
		// TODO: also consider other files, like .dockerignore, .env, etc.
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
