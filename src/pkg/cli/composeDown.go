package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
)

func ComposeDown(ctx context.Context, client client.Client) (types.ETag, error) {
	project, err := client.LoadProject()
	if err != nil {
		return "", err
	}
	term.Debug("Destroying project", project.Name)

	if DoDryRun {
		return "", ErrDryRun
	}

	return client.Destroy(ctx)
}
