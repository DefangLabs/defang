package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	pb "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func ComposeRestart(ctx context.Context, client client.Client, filePath, projectName string) ([]*pb.ServiceInfo, error) {
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
