package cli

import (
	"context"
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func ComposeDown(ctx context.Context, projectName string, fabric client.FabricClient, provider client.Provider) (types.ETag, error) {
	term.Debugf("Destroying project %q", projectName)

	// If no names are provided, destroy the entire project
	return CdCommand(ctx, projectName, provider, fabric, client.CdCommandDestroy)
}

var ErrDoNotComposeDown = errors.New("user did not want to compose down")

func InteractiveComposeDown(ctx context.Context, projectName string, fabric client.FabricClient, provider client.Provider) (types.ETag, error) {
	var wantComposeDown bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Run 'compose down' to deactivate project: " + projectName + "?",
	}, &wantComposeDown, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return "", err
	}

	track.Evt("Compose Down Prompt Answered", P("project", projectName), P("wantComposeDown", wantComposeDown))
	if !wantComposeDown {
		return "", ErrDoNotComposeDown
	}

	term.Info("Deactivating project " + projectName)
	return ComposeDown(ctx, projectName, fabric, provider)
}
