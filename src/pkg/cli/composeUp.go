package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
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

func checkDeploymentMode(prevMode, newMode modes.Mode) (modes.Mode, error) {
	// previous deployment mode | new mode          | behavior:
	// -------------------------|-------------------|-----------------------
	// any                      | unspecified       | previous mode
	// affordable               | affordable        | nop, use affordable
	// affordable               | balanced          | new mode: balanced
	// affordable               | high-availability | new mode: high-availability
	// balanced                 | affordable        | warn, use new mode: affordable
	// balanced                 | balanced          | nop, use balanced
	// balanced                 | high-availability | new mode: high-availability
	// high-availability        | affordable        | error
	// high-availability        | balanced          | warn, use balanced
	// high-availability        | high-availability | nop, use high-availability
	switch newMode {
	case modes.ModeUnspecified:
		if prevMode != modes.ModeUnspecified {
			term.Warn("No deployment mode specified; using previous deployment mode:", prevMode)
			newMode = prevMode
		}
	case modes.ModeAffordable:
		switch prevMode {
		case modes.ModeHighAvailability:
			return newMode, fmt.Errorf("will not downgrade deployment mode from %s to %s; use %s", prevMode, newMode, modes.ModeBalanced)
		case modes.ModeBalanced:
			term.Warnf("Downgrading deployment mode from %s to %s", prevMode, newMode)
		}
	case modes.ModeBalanced:
		if prevMode == modes.ModeHighAvailability {
			term.Warnf("Downgrading deployment mode from %s to %s", prevMode, newMode)
		}
	case modes.ModeHighAvailability:
		// from anything to high-availability is allowed
	}
	return newMode, nil
}

// ComposeUp validates a compose project and uploads the services using the client
func ComposeUp(ctx context.Context, fabric client.FabricClient, provider client.Provider, stack *stacks.Parameters, params ComposeUpParams) (*defangv1.DeployResponse, *compose.Project, error) {
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
		// Ignore missing configs in preview mode, because we don't want to fail the preview if some configs are missing.
		if upload != compose.UploadModeEstimate {
			if err := PrintConfigSummaryAndValidate(ctx, provider, project); err != nil {
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

	if prevUpdate, err := provider.GetProjectUpdate(ctx, project.Name); err == nil {
		prevMode := modes.Mode(prevUpdate.Mode)
		mode, err = checkDeploymentMode(prevMode, mode)
		if err != nil {
			return nil, project, err
		}
	}

	deployRequest := &client.DeployRequest{
		DeployRequest: defangv1.DeployRequest{
			Mode:           mode.Value(),
			Project:        project.Name,
			Compose:        bytes,
			DelegateDomain: delegateDomain.Zone,
		},
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

	var statesUrl, eventsUrl string
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

		if _, ok := provider.(*client.PlaygroundProvider); !ok { // Do not need upload URLs for Playground
			statesUrl, eventsUrl, err = GetStatesAndEventsUploadUrls(ctx, project.Name, provider, fabric)
			if err != nil {
				return nil, project, err
			}
			deployRequest.StatesUrl = statesUrl
			deployRequest.EventsUrl = eventsUrl
		}

		resp, err = provider.Deploy(ctx, deployRequest)
		if err != nil {
			return nil, project, err
		}
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP
	}

	err = putDeploymentAndStack(ctx, provider, fabric, stack, putDeploymentParams{
		Action:       action,
		ETag:         resp.Etag,
		Mode:         mode.Value(),
		ProjectName:  project.Name,
		ServiceCount: len(fixedProject.Services),
		StatesUrl:    statesUrl,
		EventsUrl:    eventsUrl,
	})
	if err != nil {
		term.Debug("Failed to record deployment:", err)
		term.Warn("Unable to update deployment history; deployment will proceed anyway.")
	}

	if term.DoDebug() {
		term.Println("Project:", project.Name)
		for _, serviceInfo := range resp.Services {
			PrintObject(serviceInfo.Service.Name, serviceInfo)
		}
	}
	return resp, project, nil
}
