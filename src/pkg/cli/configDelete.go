package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func ConfigDelete(ctx context.Context, client client.Client, names ...string) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}
	term.Debug(" - Deleting config", names, "in project", projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return client.DeleteConfig(ctx, &defangv1.Secrets{Names: names})
}
