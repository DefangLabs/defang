package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func ComposeDown(ctx context.Context, client client.Client) (types.ETag, error) {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return "", err
	}
	term.Debug("Destroying project", projectName)

	if DoDryRun {
		return "", ErrDryRun
	}

	return client.Destroy(ctx)
}
