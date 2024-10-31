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
func ComposeUp(ctx context.Context, loader client.Loader, c client.FabricClient, p client.Provider, upload compose.UploadMode, mode defangv1.DeploymentMode) (*defangv1.DeployResponse, *types.Project, error) {
	project, err := loader.LoadProject(ctx)
	if err != nil {
		return nil, project, err
	}

	if DoDryRun {
		upload = compose.UploadModeIgnore
	}

	listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
		configs, err := p.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
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

	// Create a new project with only the necessary resources.
	// Do not modify the original project, because the caller needs it for debugging.
	fixedProject := project.WithoutUnnecessaryResources()

	if err := compose.FixupServices(ctx, p, fixedProject, upload); err != nil {
		return nil, project, err
	}

	bytes, err := fixedProject.MarshalYAML()
	if err != nil {
		return nil, project, err
	}

	if upload == compose.UploadModeIgnore {
		fmt.Println(string(bytes))
		return nil, project, ErrDryRun
	}

	// FIXME: When do we actually do the domain delegation?
	delegateDomain, err := c.GetDelegateSubdomainZone(ctx)
	if err != nil {
		term.Debug("Failed to get delegate domain:", err)
	}

	deployRequest := &defangv1.DeployRequest{Mode: mode, Project: project.Name, Compose: bytes, DelegateDomain: delegateDomain.Zone}
	var resp *defangv1.DeployResponse
	if upload == compose.UploadModePreview {
		resp, err = p.Preview(ctx, deployRequest)
	} else {

		req := client.DelegateDomainNSServersRequest{Project: project.Name, DelegateDomain: delegateDomain.Zone}
		nsServers, err := p.DelegateDomainNSServers(ctx, req)
		if err != nil {
			return nil, project, err
		}
		if len(nsServers) > 0 {
			req := &defangv1.DelegateSubdomainZoneRequest{NameServerRecords: nsServers}
			_, err = c.DelegateSubdomainZone(ctx, req)
			if err != nil {
				return nil, project, err
			}
		}

		resp, err = p.Deploy(ctx, deployRequest)
	}
	if err != nil {
		return nil, project, err
	}

	if term.DoDebug() {
		fmt.Println("Project:", project.Name)
		for _, serviceInfo := range resp.Services {
			PrintObject(serviceInfo.Service.Name, serviceInfo)
		}
	}
	return resp, project, nil
}
