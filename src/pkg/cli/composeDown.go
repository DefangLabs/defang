package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ComposeDown(ctx context.Context, client client.Client, projectName string, names ...string) (types.ETag, error) {
	if projectName == "" {
		currentProjectName, err := client.LoadProjectName(ctx)
		if err != nil {
			return "", err
		}
		projectName = currentProjectName
	} else {
		client.SetProjectName(ctx, projectName)
	}

	term.Debugf("Destroying project %q %q", projectName, names)

	if DoDryRun {
		return "", ErrDryRun
	}

	if len(names) == 0 {
		// If no names are provided, destroy the entire project
		return client.Destroy(ctx)
	}

	resp, err := client.Delete(ctx, &defangv1.DeleteRequest{Names: names})
	if err != nil {
		return "", err
	}
	return resp.Etag, nil
}
