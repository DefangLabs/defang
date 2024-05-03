package cli

import (
	"context"
	"errors"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func ComposeStop(ctx context.Context, client client.Client) (client.ETag, error) {
	project, err := client.LoadProject()
	if err != nil {
		return "", err
	}
	if project == nil {
		return "", &ComposeError{errors.New("no project found")}
	}
	names := make([]string, 0, len(project.Services))
	for _, service := range project.Services {
		names = append(names, NormalizeServiceName(service.Name))
	}

	return Delete(ctx, client, names...)
}
