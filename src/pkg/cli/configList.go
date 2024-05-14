package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
)

func ConfigList(ctx context.Context, client client.Client) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}
	term.Debug(" - Listing config in project", projectName)

	config, err := client.ListConfig(ctx)
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
