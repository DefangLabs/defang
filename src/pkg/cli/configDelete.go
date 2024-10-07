package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigDelete(ctx context.Context, client client.Client, names ...string) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Deleting config %v in project %q", names, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	configs := make([]*defangv1.ConfigKey, len(names))
	for i, name := range names {
		configs[i] = &defangv1.ConfigKey{Name: name, Project: projectName}
	}

	return client.DeleteConfigs(ctx, &defangv1.DeleteConfigsRequest{Configs: configs})
}
