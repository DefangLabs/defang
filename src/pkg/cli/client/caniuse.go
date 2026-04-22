package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

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
		Driver:              provider.Driver(),
	})
	if err != nil {
		return err
	}

	forcedReason := resp.ForcedReason
	if resp.ForcedVersion && forcedReason == "" {
		forcedReason = "the previous CD image is incompatible with your CLI"
	}

	// Resolve each version: env override > client-side pinning > fabric response
	resp.CdImage = resolveVersion(cdOverride, resp.CdImage, preferCdVersion, "CD image", allowUpgrade, forcedReason)
	resp.PulumiVersion = resolveVersion(pulumiOverride, resp.PulumiVersion, preferPulumiVersion, "Pulumi version", allowUpgrade, forcedReason)

	provider.SetCanIUseConfig(resp)
	return nil
}

type versionLabel string

// resolveVersion picks the version to use: env override > force upgrade > allow upgrade > pin to previous > latest.
func resolveVersion(fromEnv, fromFabric, previous string, label versionLabel, allowUpgrade bool, forcedReason string) string {
	if fromEnv != "" {
		slog.Debug("Using version from env", "label", label, "version", fromEnv)
		return fromEnv
	}
	if previous == "" || fromFabric == previous {
		slog.Debug("Using version from fabric", "label", label, "version", fromFabric)
		return fromFabric
	}
	if forcedReason != "" {
		slog.Debug("Using version from fabric (forced)", "label", label, "version", fromFabric)
		slog.Warn(fmt.Sprintf("Overriding %s: %s", label, forcedReason))
		return fromFabric
	}
	if allowUpgrade {
		slog.Debug("Using latest version from fabric", "label", label, "version", fromFabric)
		slog.Info(fmt.Sprintf("Upgrading %s to latest", label))
		return fromFabric
	}
	slog.Debug("Using previous version", "label", label, "version", previous)
	slog.Warn(fmt.Sprintf("A newer %s is available; using previously deployed version. To upgrade, re-run with --allow-upgrade or set DEFANG_ALLOW_UPGRADE=1", label))
	return previous
}
