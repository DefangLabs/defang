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

func buildContext(force bool) compose.BuildContext {
	if DoDryRun {
		return compose.BuildContextIgnore
	}
	if force {
		return compose.BuildContextForce
	}
	return compose.BuildContextDigest
}

// ComposeUp validates a compose project and uploads the services using the client
func ComposeUp(ctx context.Context, c client.Client, force bool, behavior defangv1.Behavior) (*defangv1.DeployResponse, *types.Project, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, project, err
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, project, &ComposeError{err}
	}

	services, err := compose.ConvertServices(ctx, c, project.Services, buildContext(force))
	if err != nil {
		return nil, project, err
	}

	if len(services) == 0 {
		return nil, project, &ComposeError{fmt.Errorf("no services found")}
	}

	if DoDryRun {
		fmt.Println("Project:", project.Name)
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, project, ErrDryRun
	}

	for _, service := range services {
		term.Info("Deploying service", service.Name)
	}

	resp, err := c.Deploy(ctx, &defangv1.DeployRequest{
		Behavior: behavior,
		Project:  project.Name,
		Services: services,
	})
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
