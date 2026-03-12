package client

import (
	"context"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func CanIUseProvider(ctx context.Context, client FabricClient, provider Provider, projectName string, serviceCount int, allowUpgrade bool) error {
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
		Stack:             provider.GetStackName(),
	}

	resp, err := client.CanIUse(ctx, &canUseReq)
	if err != nil {
		return err
	}

	// Hard override from env vars takes absolute precedence
	cdOverride := os.Getenv("DEFANG_CD_IMAGE")
	if cdOverride != "" {
		resp.CdImage = cdOverride
	}
	pulumiOverride := os.Getenv("DEFANG_PULUMI_VERSION")
	if pulumiOverride != "" {
		resp.PulumiVersion = pulumiOverride
	}

	// Version pinning: use previous versions unless explicitly upgrading or overridden by env
	if projectName != "" && (cdOverride == "" || pulumiOverride == "") {
		if projUpdate, err := provider.GetProjectUpdate(ctx, projectName); err != nil || projUpdate == nil {
			term.Debugf("unable to get project update for %q: %v", projectName, err)
		} else {
			if cdOverride == "" {
				resp.CdImage = pinVersion(resp.CdImage, projUpdate.CdVersion, "CD image", allowUpgrade)
			}
			if pulumiOverride == "" {
				resp.PulumiVersion = pinVersion(resp.PulumiVersion, projUpdate.PulumiVersion, "Pulumi version", allowUpgrade)
			}
		}
	}

	provider.SetCanIUseConfig(resp)
	return nil
}

func pinVersion(latest, previous, label string, allowUpgrade bool) string {
	if previous == "" || latest == previous {
		return latest
	}
	if allowUpgrade {
		term.Infof("Upgrading %s from %s to %s", label, previous, latest)
		return latest
	} else {
		term.Warnf("A newer %s is available (%s); using previously deployed version (%s). To upgrade, re-run with --allow-upgrade or set DEFANG_ALLOW_UPGRADE=1", label, latest, previous)
		return previous
	}
}
