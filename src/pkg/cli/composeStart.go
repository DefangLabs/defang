package cli

import (
	"context"
	"fmt"
	"strings"

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

func buildContext(force bool) compose.BuildContext {
	if DoDryRun {
		return compose.BuildContextIgnore
	}
	if force {
		return compose.BuildContextForce
	}
	return compose.BuildContextDigest
}

// ComposeStart validates a compose project and uploads the services using the client
func ComposeStart(ctx context.Context, c client.Client, force bool) (*defangv1.DeployResponse, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, err
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, &ComposeError{err}
	}

	services, err := compose.ConvertServices(ctx, c, project.Services, buildContext(force))
	if err != nil {
		return nil, err
	}

	if len(services) == 0 {
		return nil, &ComposeError{fmt.Errorf("no services found")}
	}

	if DoDryRun {
		for _, service := range services {
			PrintObject(service.Name, service)
		}
		return nil, ErrDryRun
	}

	for _, service := range services {
		term.Info("Deploying service", service.Name)
	}

	resp, err := c.Deploy(ctx, &defangv1.DeployRequest{
		Services: services,
	})
	if err != nil {
		if strings.Contains(err.Error(), "missing configs") {
			// Extract the list of missing configs
			formattedMissingConfigs := strings.Join(strings.Fields(err.Error()[strings.Index(err.Error(), "[")+1:strings.Index(err.Error(), "]")]), ", ")
			return nil, fmt.Errorf("missing configs: run defang config set on %s", formattedMissingConfigs)
		}
		return nil, fmt.Errorf("deployment failed: %w", err) // Wrap the original error with a new message
	}

	if term.DoDebug() {
		for _, service := range resp.Services {
			PrintObject(service.Service.Name, service)
		}
	}
	return resp, nil
}
