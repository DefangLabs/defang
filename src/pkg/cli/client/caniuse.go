package client

import (
	"context"
	"errors"
	"os"

	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func CanIUseProvider(ctx context.Context, client FabricClient, provider Provider, projectName string, serviceCount int, allowUpgrade bool) error {
	info, err := provider.AccountInfo(ctx)
	if err != nil {
		return err
	}

	cdOverride := os.Getenv("DEFANG_CD_IMAGE")
	pulumiOverride := os.Getenv("DEFANG_PULUMI_VERSION")

	// Look up previously deployed versions to send to fabric and for client-side pinning
	var prevCD, prevPulumi string
	if projectName != "" && !allowUpgrade && (cdOverride == "" || pulumiOverride == "") {
		if prevUpdate, err := provider.GetProjectUpdate(ctx, projectName); err != nil {
			if !errors.Is(err, ErrNotExist) {
				return err
			}
			// New project, no previous versions to pin to
		} else {
			prevCD = prevUpdate.CdVersion
			prevPulumi = prevUpdate.PulumiVersion
		}
	}

	resp, err := client.CanIUse(ctx, &defangv1.CanIUseRequest{
		Project:           projectName,
		Provider:          info.Provider.Value(),
		ProviderAccountId: info.AccountID,
		Region:            info.Region,
		ServiceCount:      int32(serviceCount), // #nosec G115 - service count will not overflow int32
		Stack:             provider.GetStackName(),
		CdVersion:         prevCD,
		PulumiVersion:     prevPulumi,
	})
	if err != nil {
		return err
	}

	// Resolve each version: env override > client-side pinning > fabric response
	resp.CdImage = resolveVersion(cdOverride, resp.CdImage, prevCD, "CD image", allowUpgrade)
	resp.PulumiVersion = resolveVersion(pulumiOverride, resp.PulumiVersion, prevPulumi, "Pulumi version", allowUpgrade)

	provider.SetCanIUseConfig(resp)
	return nil
}

// resolveVersion picks the version to use given an env override, the fabric's response, and the previously deployed version.
func resolveVersion(envOverride, latest, previous, label string, allowUpgrade bool) string {
	if envOverride != "" {
		return envOverride
	}
	return pinVersion(latest, previous, label, allowUpgrade)
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
