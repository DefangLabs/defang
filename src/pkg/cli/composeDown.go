package cli

import (
	"context"
	"errors"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ComposeDown(ctx context.Context, loader client.Loader, client client.FabricClient, provider client.Provider, names ...string) (types.ETag, error) {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return "", err
	}

	term.Debugf("Destroying project %q %q", projectName, names)

	if DoDryRun {
		return "", ErrDryRun
	}

	if len(names) == 0 {
		accountInfo, err := provider.AccountInfo(ctx)
		if err != nil {
			return "", err
		}

		// If no names are provided, destroy the entire project
		etag, err := provider.Destroy(ctx, &defangv1.DestroyRequest{Project: projectName})
		if err != nil {
			return "", err
		}

		err = client.PutDeployment(ctx, &defangv1.PutDeploymentRequest{
			Deployment: &defangv1.Deployment{
				Action:            defangv1.DeploymentAction_DEPLOYMENT_ACTION_DOWN,
				Id:                etag,
				Project:           projectName,
				Provider:          string(accountInfo.Provider()),
				ProviderAccountId: accountInfo.AccountID(),
				Timestamp:         timestamppb.New(time.Now()),
			},
		})

		if err != nil {
			term.Debug("PutDeployment failed:", err)
		}

		return etag, nil
	}

	delegateDomain, err := client.GetDelegateSubdomainZone(ctx)
	if err != nil {
		term.Debug("Failed to get delegate domain:", err)
	}

	resp, err := provider.Delete(ctx, &defangv1.DeleteRequest{Project: projectName, Names: names, DelegateDomain: delegateDomain.Zone})
	if err != nil {
		return "", err
	}

	return resp.Etag, nil
}

var ErrDoNotComposeDown = errors.New("user did not want to compose down")

func InteractiveComposeDown(ctx context.Context, provider client.Provider, projectName string) (types.ETag, error) {
	var wantComposeDown bool
	if err := survey.AskOne(&survey.Confirm{
		Message: "Run 'compose down' to deactivate project: " + projectName + "?",
	}, &wantComposeDown); err != nil {
		return "", err
	}

	if !wantComposeDown {
		return "", ErrDoNotComposeDown
	}

	term.Info("Deactivating project " + projectName)
	return provider.Destroy(ctx, &defangv1.DestroyRequest{Project: projectName})
}
