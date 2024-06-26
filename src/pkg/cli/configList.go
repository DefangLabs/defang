package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func ConfigList(ctx context.Context, client client.Client) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Listing config in project %q", projectName)

	config, err := client.ListConfig(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
