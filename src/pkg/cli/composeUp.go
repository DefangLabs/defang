package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/compose-spec/compose-go/v2/types"
	"google.golang.org/protobuf/types/known/structpb"
	"gopkg.in/yaml.v3"
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
func ComposeUp(ctx context.Context, c client.Client, force bool, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *types.Project, error) {
	project, err := c.LoadProject(ctx)
	if err != nil {
		return nil, project, err
	}

	if err := compose.ValidateProject(project); err != nil {
		return nil, project, &ComposeError{err}
	}

	if err := compose.FixupServices(ctx, c, project.Services, buildContext(force)); err != nil {
		return nil, project, err
	}

	bytes, err := project.MarshalYAML()
	if err != nil {
		return nil, project, err
	}

	if DoDryRun {
		fmt.Println(string(bytes))
		return nil, project, ErrDryRun
	}

	// Unmarshal the project into a map so we can convert it to a structpb.Struct
	var asMap map[string]any
	if err := yaml.Unmarshal(bytes, &asMap); err != nil {
		return nil, project, err
	}

	strpb, err := structpb.NewStruct(asMap)
	if err != nil {
		return nil, project, err
	}

	for _, service := range project.Services {
		term.Info("Deploying service", service.Name)
	}

	resp, err := c.Deploy(ctx, &defangv1.DeployRequest{
		Mode:    mode,
		Project: project.Name,
		Compose: strpb,
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
