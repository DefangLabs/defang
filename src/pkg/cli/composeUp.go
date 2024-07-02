package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
)

type ComposeError struct {
	error
}

func (e ComposeError) Unwrap() error {
	return e.error
}

// ComposeUp validates a compose project and uploads the services using the client
func ComposeUp(ctx context.Context, c client.Client, force compose.BuildContext, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *types.Project, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, project, err
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, project, &ComposeError{err}
	}

	services, err := compose.ConvertServices(ctx, c, project.Services, force)
	if err != nil {
		return nil, project, err
	}

	if len(services) == 0 {
		return nil, project, &ComposeError{fmt.Errorf("no services found")}
	}

	if force == compose.BuildContextIgnore {
		fmt.Println("Project:", project.Name)
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, project, ErrDryRun
	}

	for _, service := range services {
		term.Info("Deploying service", service.Name)
	}

	var resp *defangv1.DeployResponse
	if force == compose.BuildContextPreview {
		resp, err = c.Preview(ctx, &defangv1.DeployRequest{Mode: mode, Project: project.Name, Services: services})
	} else {
		resp, err = c.Deploy(ctx, &defangv1.DeployRequest{Mode: mode, Project: project.Name, Services: services})
	}
	if err != nil {
		return nil, project, err
	}

	if term.DoDebug() {
		fmt.Println("Project:", project.Name)
		for _, service := range resp.Services {
			PrintObject(service.Service.Name, service)
		}
	}
	return resp, project, nil
}
