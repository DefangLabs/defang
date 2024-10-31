package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func ConfigList(ctx context.Context, loader compose.Loader, provider client.Provider) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing config in project %q", projectName)

	config, err := provider.ListConfig(ctx, projectName)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
