package cli

import (
	"context"
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ComposeDown(ctx context.Context, provider client.Provider, projectName string, names ...string) (types.ETag, error) {
	if projectName == "" {
		currentProjectName, err := provider.LoadProjectName(ctx)
		if err != nil {
			return "", err
		}
		projectName = currentProjectName
	} else {
		provider.SetProjectName(projectName)
	}

	term.Debugf("Destroying project %q %q", projectName, names)

	if DoDryRun {
		return "", ErrDryRun
	}

	if len(names) == 0 {
		// If no names are provided, destroy the entire project
		return provider.Destroy(ctx)
	}

	resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}

var ErrDoNotComposeDown = errors.New("user did not want to compose down")

func InteractiveComposeDown(ctx context.Context, provider client.Provider, projectName string) (types.ETag, error) {
	var wantComposeDown bool
	err := survey.AskOne(&survey.Confirm{
		Message: "Run 'compose down' to deactivate project: " + projectName + "?",
	}, &wantComposeDown)

	if err != nil {
		return "", err
	}

	if !wantComposeDown {
		return "", ErrDoNotComposeDown
	}

	term.Info("Deactivating project " + projectName)
	return ComposeDown(ctx, provider, projectName)
}
