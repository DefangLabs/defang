package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ComposeError struct {
	error
}

func (e ComposeError) Unwrap() error {
	return e.error
}

// ComposeStart validates a compose project and uploads the services using the client
func ComposeStart(ctx context.Context, c client.Client, force compose.BuildContext) (*defangv1.DeployResponse, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, &ComposeError{err}
	}

	services, err := compose.ConvertServices(ctx, c, project.Services, force)
	if err != nil {
		return nil, err
	}

	if len(services) == 0 {
		return nil, &ComposeError{fmt.Errorf("no services found")}
	}

	if force == compose.BuildContextIgnore {
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, ErrDryRun
	}

	for _, service := range services {
		term.Info("Deploying service", service.Name)
	}

	var resp *defangv1.DeployResponse
	if force == compose.BuildContextPreview {
		resp, err = c.Preview(ctx, &defangv1.DeployRequest{Services: services})
	} else {
		resp, err = c.Deploy(ctx, &defangv1.DeployRequest{Services: services})
	}
	if err != nil {
		return nil, err
	}

	if term.DoDebug() {
		for _, service := range resp.Services {
			PrintObject(service.Service.Name, service)
		}
	}
	return resp, nil
}
