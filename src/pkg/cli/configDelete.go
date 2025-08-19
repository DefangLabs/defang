package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigDelete(ctx context.Context, projectName string, provider client.Provider, names ...string) error {
	term.Debugf("Deleting config %v in project %q", names, projectName)

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	return provider.DeleteConfig(ctx, &defangv1.Secrets{Names: names, Project: projectName})
}
