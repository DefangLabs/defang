package command

import (
	"os"
	"strings"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/session"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

func NewCommandSession(cmd *cobra.Command) (*session.Session, error) {
	ctx := cmd.Context()
	options := NewSessionLoaderOptionsForCommand(cmd)
	sessionLoader := session.NewSessionLoader(global.Client, ec, options)
	return sessionLoader.LoadSession(ctx)
}

func NewSessionLoaderOptionsForCommand(cmd *cobra.Command) session.SessionLoaderOptions {
	stack, err := cmd.Flags().GetString("stack")
	if err != nil {
		panic(err)
	}

	configPaths, _ := cmd.Flags().GetStringArray("file")
	provider, _ := cmd.Flag("provider").Value.(*client.ProviderID)
	projectName, _ := cmd.Flags().GetString("project-name")

	if slash := strings.Index(projectName, "/"); slash != -1 {
		// Compose project names cannot have slashes; use the part after the slash as the "stack" name
		stackName := projectName[slash+1:]
		term.Debugf("Setting DEFANG_SUFFIX=%q", stackName)
		os.Setenv("DEFANG_SUFFIX", stackName)
		projectName = projectName[:slash]
	}

	// Avoid common mistakes
	if provider.Set(projectName) == nil && !cmd.Flag("provider").Changed {
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
	return session.SessionLoaderOptions{
		Stack:            stack,
		ProviderID:       *provider,
		ComposeFilePaths: configPaths,
		ProjectName:      projectName,
		Interactive:      !global.NonInteractive,
	}
}
