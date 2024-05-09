package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	"github.com/defang-io/defang/src/pkg/types"
)

func ComposeDown(ctx context.Context, client client.Client) (types.ETag, error) {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return "", err
	}
	term.Debug(" - Destroying project", projectName)
	return client.Destroy(ctx)
}
