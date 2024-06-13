package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
)

func ConfigList(ctx context.Context, client client.Client) error {
	project, err := client.LoadProject()
	if err != nil {
		return err
	}
	term.Debug("Listing config in project", project.Name)

	config, err := client.ListConfig(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
