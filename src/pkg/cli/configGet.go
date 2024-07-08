package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigGet(ctx context.Context, client client.Client, names ...string) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("get config %v in project %q", names, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	config, err := client.GetConfig(ctx, &defangv1.Configs{Names: names, Project: projectName})
	if err != nil {
		return err
	}

	PrintConfigData(config)

	return nil
}
