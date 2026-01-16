package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/spf13/cobra"
)

func NewCommandSession(cmd *cobra.Command) (*session.Session, error) {
	ctx := cmd.Context()
	options := NewSessionLoaderOptionsForCommand(cmd)
	sm, err := newStackManagerForCommand(cmd)
	if err != nil {
		return nil, err
	}
	sessionLoader := session.NewSessionLoader(global.Client, ec, sm, options)
	session, err := sessionLoader.LoadSession(ctx)
	if err != nil {
		return nil, err
	}
	_, err = session.Provider.AccountInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get account info from provider %q: %w", session.Stack.Provider, err)
	}
	return session, nil
}

func NewSessionLoaderOptionsForCommand(cmd *cobra.Command) session.SessionLoaderOptions {
	stack, _ := cmd.Flags().GetString("stack")
	configPaths, _ := cmd.Flags().GetStringArray("file")
	provider, _ := cmd.Flag("provider").Value.(*client.ProviderID)
	projectName, _ := cmd.Flags().GetString("project-name")

	// Avoid common mistakes
	if projectName != "" {
		var maybeProvider client.ProviderID
		if maybeProvider.Set(projectName) == nil && !cmd.Flag("provider").Changed {
			// using -p with a provider name instead of -P
			term.Warnf("Project name %q looks like a provider name; did you mean to use -P=%s instead of -p?", projectName, projectName)
			doubleCheckProjectName(projectName)
		} else if strings.HasPrefix(projectName, "roject-name") {
			// -project-name= instead of --project-name
			term.Warn("Did you mean to use --project-name instead of -project-name?")
			doubleCheckProjectName(projectName)
		} else if strings.HasPrefix(projectName, "rovider") {
			// -provider= instead of --provider
			term.Warn("Did you mean to use --provider instead of -provider?")
			doubleCheckProjectName(projectName)
		}
	}
	return session.SessionLoaderOptions{
		Stack:            stack,
		ProviderID:       *provider,
		ComposeFilePaths: configPaths,
		ProjectName:      projectName,
		Interactive:      !global.NonInteractive,
	}
}

func doubleCheckProjectName(projectName string) {
	if global.NonInteractive {
		return
	}
	var confirm bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Continue with project: " + projectName + "?",
	}, &confirm, survey.WithStdio(term.DefaultTerm.Stdio()))
	track.Evt("ProjectNameConfirm", P("project", projectName), P("confirm", confirm), P("err", err))
	if err == nil && !confirm {
		os.Exit(1)
	}
}

func newStackManagerForCommand(cmd *cobra.Command) (session.StacksManager, error) {
	loader := configureLoader(cmd)
	projectName, _ := cmd.Flags().GetString("project-name")
	if projectName == "" {
		var err error
		projectName, err = loader.LoadProjectName(cmd.Context())
		if err != nil {
			term.Debugf("Unable to load project name: %v", err)
		}
	}
	targetDirectory, err := findTargetDirectory()
	if err != nil {
		// No .defang folder in any parent directory, so we'd need a project-name to fetch stacks from Fabric
		if projectName == "" {
			if errors.Is(err, os.ErrNotExist) {
				return nil, errors.New("no local stack files found; create a new stack or use --project-name to load known stacks")
			}
			return nil, err
		}
		// Use empty string for targetDirectory when outside project but projectName is provided
		targetDirectory = loader.TargetDirectory()
	}
	sm, err := stacks.NewManager(global.Client, targetDirectory, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to create stack manager: %w", err)
	}
	return sm, nil
}

func findTargetDirectory() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	for {
		info, err := os.Stat(filepath.Join(wd, stacks.Directory))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("failed to stat .defang directory: %w", err)
			}
		} else if info.IsDir() {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			// reached root directory
			return "", os.ErrNotExist
		}
		wd = parent
	}
}
