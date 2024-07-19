package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigGet(ctx context.Context, client client.Client, names ...string) (types.ConfigData, error) {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return nil, err
	}
	term.Debugf("get config %v in project %q", names, projectName)

	if DoDryRun {
		return nil, ErrDryRun
	}

	return client.GetConfig(ctx, &defangv1.Configs{})
}
