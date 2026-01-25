package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/spf13/cobra"
)

type commandSessionOpts struct {
	CheckAccountInfo bool
}

func newCommandSession(cmd *cobra.Command) (*session.Session, error) {
	return newCommandSessionWithOpts(cmd, commandSessionOpts{
		CheckAccountInfo: true,
	})
}

func newCommandSessionWithOpts(cmd *cobra.Command, opts commandSessionOpts) (*session.Session, error) {
	ctx := cmd.Context()

	options := newSessionLoaderOptionsForCommand(cmd)
	sm, err := newStackManagerForLoader(ctx, configureLoader(cmd))
	if err != nil {
		term.Debugf("Could not create stack manager: %v", err)
	}
	sessionLoader := session.NewSessionLoader(global.Client, sm, options)
	session, err := sessionLoader.LoadSession(ctx)
	if err != nil {
		return nil, err
	}
	if opts.CheckAccountInfo {
		_, err = session.Provider.AccountInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get account info from provider %q: %w", session.Stack.Provider, err)
		}
	}
	return session, nil
}

func newSessionLoaderOptionsForCommand(cmd *cobra.Command) session.SessionLoaderOptions {
	configPaths, _ := cmd.Flags().GetStringArray("file")
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
		ComposeFilePaths: configPaths,
		ProjectName:      projectName,
		GetStackOpts: stacks.GetStackOpts{
			Interactive: !global.NonInteractive,
			Default:     global.Stack,
		},
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

func newStackManagerForLoader(ctx context.Context, loader *compose.Loader) (session.StacksManager, error) {
	targetDirectory, err := findTargetDirectory()
	if err != nil {
		targetDirectory = loader.TargetDirectory()
	}
	projectName, _, err := loader.LoadProjectName(ctx)
	if err != nil {
		term.Debugf("Could not determine project name: %v", err)
	}
	sm, err := stacks.NewManager(global.Client, targetDirectory, projectName, ec)
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
