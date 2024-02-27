package cli

import (
	"context"
	"errors"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/defang-io/defang/src/pkg/cli/client"
	v1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func ComposeRestart(ctx context.Context, client client.Client, project *composeTypes.Project) ([]*v1.ServiceInfo, error) {
	if project == nil {
		return nil, &ComposeError{errors.New("no project found")}
	}
	names := make([]string, 0, len(project.Services))
	for _, service := range project.Services {
		names = append(names, NormalizeServiceName(service.Name))
	}

	return Restart(ctx, client, names...)
}
