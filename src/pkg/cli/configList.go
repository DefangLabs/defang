package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigList(ctx context.Context, loader client.Loader, provider client.Provider) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing config in project %q", projectName)

	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
	if err != nil {
		return err
	}

	return PrintObject("", config)
}
