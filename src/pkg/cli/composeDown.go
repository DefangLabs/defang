package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
)

func ComposeDown(ctx context.Context, client client.Client, filePath, projectName string) (string, error) {
	// resp, err := client.Deploy(ctx, &pb.DeployRequest{})
	// return resp.Etag, err

	project, err := loadDockerCompose(filePath, projectName)
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(project.Services))
	for _, service := range project.Services {
		names = append(names, NormalizeServiceName(service.Name))
	}

	return Delete(ctx, client, names...)
}
