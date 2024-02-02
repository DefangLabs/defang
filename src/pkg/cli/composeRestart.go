package cli

import (
	"context"

	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
	"github.com/defang-io/defang/src/protos/io/defang/v1/defangv1connect"
)

func ComposeRestart(ctx context.Context, client defangv1connect.FabricControllerClient, filePath, projectName string) ([]*v1.ServiceInfo, error) {
	project, err := loadDockerCompose(filePath, projectName)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(project.Services))
	for _, service := range project.Services {
		names = append(names, NormalizeServiceName(service.Name))
	}

	return Restart(ctx, client, names...)
}
