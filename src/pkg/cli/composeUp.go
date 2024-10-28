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
func ComposeUp(ctx context.Context, c client.Client, upload compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *types.Project, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, project, err
	}

	listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
		configs, err := c.ListConfig(ctx)
		if err != nil {
			return nil, err
		}

		return configs.Names, nil
	}

	if err := compose.ValidateProjectConfig(ctx, project, listConfigNamesFunc); err != nil {
		return nil, project, &ComposeError{err}
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, project, &ComposeError{err}
	}

	if err := compose.FixupServices(ctx, c, project.Services, upload); err != nil {
		return nil, project, err
	}

	bytes, err := project.MarshalYAML()
	if err != nil {
		return nil, project, err
	}

	if upload == compose.UploadModeIgnore {
		fmt.Println(string(bytes))
		return nil, project, ErrDryRun
	}

	deployRequest := &defangv1.DeployRequest{Mode: mode, Project: project.Name, Compose: bytes}
	var resp *defangv1.DeployResponse
	if upload == compose.UploadModePreview {
		resp, err = c.Preview(ctx, deployRequest)
	} else {
		resp, err = c.Deploy(ctx, deployRequest)
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
