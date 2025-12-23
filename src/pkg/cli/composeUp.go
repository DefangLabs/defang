package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ComposeError struct {
	error
}

func (e ComposeError) Unwrap() error {
	return e.error
}

type ComposeUpParams struct {
	Project    *compose.Project
	UploadMode compose.UploadMode
	Mode       modes.Mode
}

// ComposeUp validates a compose project and uploads the services using the client
func ComposeUp(ctx context.Context, fabric client.FabricClient, provider client.Provider, params ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
	upload := params.UploadMode
	project := params.Project
	mode := params.Mode

	if dryrun.DoDryRun {
		upload = compose.UploadModeIgnore
	}

	// Validate Dockerfiles before processing the build contexts
	// Only validate when actually deploying (not for dry-run/ignore mode)
	if upload != compose.UploadModeIgnore && upload != compose.UploadModeEstimate {
		if err := compose.ValidateServiceDockerfiles(project); err != nil {
			return nil, project, &ComposeError{err}
		}
	}

	// Validate the project configuration against the provider's configuration, but only if we are going to deploy.
	// FIXME: should not need to validate configs if we are doing preview, but preview will fail on missing configs.
	if upload != compose.UploadModeIgnore {
		listConfigNamesFunc := func(ctx context.Context) ([]string, error) {
			configs, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: project.Name})
			if err != nil {
				return nil, err
			}

			return configs.Names, nil
		}

		// Ignore missing configs in preview mode, because we don't want to fail the preview if some configs are missing.
		if upload != compose.UploadModeEstimate {
			listConfigNames, err := listConfigNamesFunc(ctx)
			if err != nil {
				return nil, project, err
			}

			if err := compose.ValidateProjectConfig(ctx, project, listConfigNames); err != nil {
				return nil, project, &ComposeError{err}
			}

			// Print config resolution summary
			err = PrintConfigResolutionSummary(project, listConfigNames)
			if err != nil {
				return nil, project, err
			}
		}
	}

	// Create a new project with only the necessary resources.
	// Do not modify the original project, because the caller needs it for debugging.
	fixedProject := project.WithoutUnnecessaryResources()
	if err := compose.FixupServices(ctx, provider, fixedProject, upload); err != nil {
		return nil, project, err
	}

	if err := compose.ValidateProject(fixedProject, mode); err != nil {
		return nil, project, &ComposeError{err}
	}

	bytes, err := fixedProject.MarshalYAML()
	if err != nil {
		return nil, project, err
	}

	if upload == compose.UploadModeIgnore {
		term.Println(string(bytes))
		return nil, project, dryrun.ErrDryRun
	}

	delegateDomain, err := fabric.GetDelegateSubdomainZone(ctx, &defangv1.GetDelegateSubdomainZoneRequest{
		Project: project.Name,
		Stack:   provider.GetStackNameForDomain(),
	})
	if err != nil {
		term.Debug("GetDelegateSubdomainZone failed:", err)
		return nil, project, errors.New("failed to get delegate domain")
	}

	deployRequest := &defangv1.DeployRequest{
		Mode:           mode.Value(),
		Project:        project.Name,
		Compose:        bytes,
		DelegateDomain: delegateDomain.Zone,
	}

	delegation, err := provider.PrepareDomainDelegation(ctx, client.PrepareDomainDelegationRequest{
		DelegateDomain: delegateDomain.Zone,
		Preview:        upload == compose.UploadModePreview || upload == compose.UploadModeEstimate,
		Project:        project.Name,
	})
	if err != nil {
		return nil, project, err
	} else if delegation != nil {
		deployRequest.DelegationSetId = delegation.DelegationSetId
	}

	accountInfo, err := provider.AccountInfo(ctx)
	if err != nil {
		return nil, project, err
	}

	var action defangv1.DeploymentAction
	var resp *defangv1.DeployResponse
	if upload == compose.UploadModePreview || upload == compose.UploadModeEstimate {
		resp, err = provider.Preview(ctx, deployRequest)
		if err != nil {
			return nil, project, err
		}
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_PREVIEW
	} else {
		if delegation != nil && len(delegation.NameServers) > 0 {
			req := &defangv1.DelegateSubdomainZoneRequest{
				NameServerRecords: delegation.NameServers,
				Project:           project.Name,
				Stack:             provider.GetStackNameForDomain(),
			}
			_, err = fabric.CreateDelegateSubdomainZone(ctx, req)
			if err != nil {
				return nil, project, err
			}
		}

		resp, err = provider.Deploy(ctx, deployRequest)
		if err != nil {
			return nil, project, err
		}
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP
	}

	err = fabric.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
		Deployment: &defangv1.Deployment{
			Action:            action,
			Id:                resp.Etag,
			Project:           project.Name,
			Provider:          accountInfo.Provider.Value(),
			ProviderAccountId: accountInfo.AccountID,
			ProviderString:    string(accountInfo.Provider),
			Region:            accountInfo.Region,
			ServiceCount:      int32(len(fixedProject.Services)), // #nosec G115 - service count will not overflow int32
			Stack:             provider.GetStackName(),
			Timestamp:         timestamppb.Now(),
			Mode:              mode.Value(),
		},
	})
	if err != nil {
		term.Debugf("PutDeployment failed: %v", err)
		term.Warn("Unable to update deployment history, but deployment will proceed anyway.")
	}

	if term.DoDebug() {
		term.Println("Project:", project.Name)
		for _, serviceInfo := range resp.Services {
			PrintObject(serviceInfo.Service.Name, serviceInfo)
		}
	}
	return resp, project, nil
}
