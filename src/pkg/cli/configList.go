package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func ConfigList(ctx context.Context, provider client.Provider) error {
	projectName, err := provider.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Listing config in project %q", projectName)

	config, err := provider.ListConfig(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
