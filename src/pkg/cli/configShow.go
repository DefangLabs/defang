package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigShow(ctx context.Context, client client.Client, names ...string) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Show config %v in project %q", names, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return client.ShowConfig(ctx, &defangv1.Secrets{})
}
