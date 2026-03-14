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
	var preferCdVersion, preferPulumiVersion string
	if projectName != "" && !allowUpgrade && (cdOverride == "" || pulumiOverride == "") {
		if prevUpdate, err := provider.GetProjectUpdate(ctx, projectName); err != nil {
			if !errors.Is(err, ErrNotExist) {
				return err
			}
			// New project, no previous versions to pin to
		} else {
			preferCdVersion = prevUpdate.CdVersion
			preferPulumiVersion = prevUpdate.PulumiVersion
		}
	}

	resp, err := client.CanIUse(ctx, &defangv1.CanIUseRequest{
		Project:             projectName,
		Provider:            info.Provider.Value(),
		ProviderAccountId:   info.AccountID,
		Region:              info.Region,
		ServiceCount:        int32(serviceCount), // #nosec G115 - service count will not overflow int32
		Stack:               provider.GetStackName(),
		PreferCdVersion:     preferCdVersion,
		PreferPulumiVersion: preferPulumiVersion,
	})
	if err != nil {
		return err
	}

	// Resolve each version: env override > client-side pinning > fabric response
	resp.CdImage = resolveVersion(cdOverride, resp.CdImage, preferCdVersion, "CD image", allowUpgrade, resp.ForcedVersion)
	resp.PulumiVersion = resolveVersion(pulumiOverride, resp.PulumiVersion, preferPulumiVersion, "Pulumi version", allowUpgrade, resp.ForcedVersion)

	provider.SetCanIUseConfig(resp)
	return nil
}

type versionLabel string

// resolveVersion picks the version to use: env override > force upgrade > allow upgrade > pin to previous > latest.
func resolveVersion(fromEnv, fromServer, previous string, label versionLabel, allowUpgrade, serverForced bool) string {
	if fromEnv != "" {
		term.Debugf("Using %s from env: %s", label, fromEnv)
		return fromEnv
	}
	if previous == "" || fromServer == previous {
		term.Debugf("Using %s: %s", label, fromServer)
		return fromServer
	}
	if serverForced {
		term.Debugf("Using %s from server: %s", label, fromServer)
		term.Warnf("Force-upgrading %s...", label)
		return fromServer
	}
	if allowUpgrade {
		term.Debugf("Using latest %s: %s", label, fromServer)
		term.Infof("Upgrading %s...", label)
		return fromServer
	}
	term.Debugf("Using previous %s: %s", label, previous)
	term.Warnf("A newer %s is available; using previously deployed version. To upgrade, re-run with --allow-upgrade or set DEFANG_ALLOW_UPGRADE=1", label)
	return previous
}
