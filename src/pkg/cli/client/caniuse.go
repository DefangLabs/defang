package client

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func CanIUseProvider(ctx context.Context, client FabricClient, provider Provider, projectName, stack string, serviceCount int) error {
	info, err := provider.AccountInfo(ctx)
	if err != nil {
		return err
	}

	canUseReq := defangv1.CanIUseRequest{
		Project:           projectName,
		Provider:          info.Provider.Value(),
		ProviderAccountId: info.AccountID,
		Region:            info.Region,
		ServiceCount:      int32(serviceCount), // #nosec G115 - service count will not overflow int32
		Stack:             stack,
	}

	resp, err := client.CanIUse(ctx, &canUseReq)
	if err != nil {
		return err
	}

	// Allow local override of the CD image and Pulumi version
	resp.CdImage = pkg.Getenv("DEFANG_CD_IMAGE", resp.CdImage)
	resp.PulumiVersion = pkg.Getenv("DEFANG_PULUMI_VERSION", resp.PulumiVersion)
	if resp.CdImage == "previous" {
		if projUpdate, err := provider.GetProjectUpdate(ctx, projectName); err != nil {
			term.Debugf("unable to get project update for project %s: %v", projectName, err)
		} else if projUpdate != nil {
			resp.CdImage = projUpdate.CdVersion
		}
	}
	provider.SetCanIUseConfig(resp)
	return nil
}
