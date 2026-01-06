package cli

import (
	"context"
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ComposeDown(ctx context.Context, projectName string, fabric client.FabricClient, provider client.Provider, names ...string) (types.ETag, error) {
	term.Debugf("Destroying project %q %q", projectName, names)

	if len(names) == 0 {
		// If no names are provided, destroy the entire project
		return CdCommand(ctx, projectName, provider, fabric, "destroy")
	}

	// If names are provided, treat it as a delete = partial update
	delegateDomain, err := fabric.GetDelegateSubdomainZone(ctx, &defangv1.GetDelegateSubdomainZoneRequest{
		Project: projectName,
		Stack:   provider.GetStackNameForDomain(),
	})
	if err != nil {
		term.Debug("GetDelegateSubdomainZone failed:", err)
		return "", errors.New("failed to get delegate domain")
	}

	if dryrun.DoDryRun {
		return "", dryrun.ErrDryRun
	}

	resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names, DelegateDomain: delegateDomain.Zone})
	if err != nil {
		return "", err
	}

	err = putDeployment(ctx, provider, fabric, putDeploymentParams{
		Action:      defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP, // update
		ETag:        resp.Etag,
		ProjectName: projectName,
	})
	if err != nil {
		term.Debug("Failed to record deployment:", err)
		term.Warn("Unable to update deployment history; deployment will proceed anyway.")
	}

	return resp.Etag, nil
}

var ErrDoNotComposeDown = errors.New("user did not want to compose down")

func InteractiveComposeDown(ctx context.Context, projectName string, fabric client.FabricClient, provider client.Provider) (types.ETag, error) {
	var wantComposeDown bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Run 'compose down' to deactivate project: " + projectName + "?",
	}, &wantComposeDown, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return "", err
	}

	track.Evt("Compose Down Prompt Answered", P("project", projectName), P("wantComposeDown", wantComposeDown))
	if !wantComposeDown {
		return "", ErrDoNotComposeDown
	}

	term.Info("Deactivating project " + projectName)
	return ComposeDown(ctx, projectName, fabric, provider)
}
