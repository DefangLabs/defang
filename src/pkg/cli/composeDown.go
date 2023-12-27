package cli

import (
	"context"

	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func ComposeDown(ctx context.Context, client defangv1connect.FabricControllerClient, filePath, projectName string) (string, error) {
	project, err := loadDockerCompose(filePath, projectName)
	if err != nil {
		return "", err
	}

	names := make([]string, 0, len(project.Services))
	for _, service := range project.Services {
		names = append(names, service.Name)
	}

	return Delete(ctx, client, names...)
}
