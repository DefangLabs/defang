package cli

import (
	"context"
	"errors"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ComposeDown(ctx context.Context, projectName string, fabric client.FabricClient, provider client.Provider, names ...string) (types.ETag, error) {
	term.Debugf("Destroying project %q %q", projectName, names)

	accountInfo, err := provider.AccountInfo(ctx)
	if err != nil {
		return "", err
	}

	if dryrun.DoDryRun {
		return "", dryrun.ErrDryRun
	}

	var etag types.ETag
	var action defangv1.DeploymentAction
	if len(names) == 0 {
		// If no names are provided, destroy the entire project
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN

		etag, err = provider.Destroy(ctx, &defangv1.DestroyRequest{Project: projectName})
		if err != nil {
			return "", err
		}

		err = fabric.DeleteSubdomainZone(ctx, &defangv1.DeleteSubdomainZoneRequest{
			Project: projectName,
			Stack:   provider.GetStackNameForDomain(),
		})
		if err != nil {
			term.Debug("DeleteSubdomainZone failed:", err)
		}
	} else {
		// If names are provided, treat it as a delete = partial update
		action = defangv1.DeploymentAction_DEPLOYMENT_ACTION_UP

		delegateDomain, err := fabric.GetDelegateSubdomainZone(ctx, &defangv1.GetDelegateSubdomainZoneRequest{
			Project: projectName,
			Stack:   provider.GetStackNameForDomain(),
		})
		if err != nil {
			term.Debug("GetDelegateSubdomainZone failed:", err)
			return "", errors.New("failed to get delegate domain")
		}

		resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names, DelegateDomain: delegateDomain.Zone})
		if err != nil {
			return "", err
		}
		etag = resp.Etag
	}

	err = fabric.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
		Deployment: &defangv1.Deployment{
			Action:            action,
			Id:                etag,
			Project:           projectName,
			Provider:          accountInfo.Provider.Value(),
			ProviderAccountId: accountInfo.AccountID,
			ProviderString:    string(accountInfo.Provider),
			Region:            accountInfo.Region,
			Stack:             provider.GetStackName(),
			Timestamp:         timestamppb.New(time.Now()),
		},
	})
	if err != nil {
		term.Debug("PutDeployment failed:", err)
		term.Warn("Unable to update deployment history, but deployment will proceed anyway.")
	}

	return etag, nil
}

var ErrDoNotComposeDown = errors.New("user did not want to compose down")

func InteractiveComposeDown(ctx context.Context, provider client.Provider, projectName string) (types.ETag, error) {
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
	return provider.Destroy(ctx, &defangv1.DestroyRequest{Project: projectName})
}
