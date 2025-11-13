package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigGet(ctx context.Context, projectName string, configName []string, provider client.Provider) (*defangv1.GetConfigsResponse, error) {
	term.Debugf("Getting config %q", configName)

	if dryrun.DoDryRun {
		return &defangv1.GetConfigsResponse{}, dryrun.ErrDryRun
	}

	var configKeys []*defangv1.ConfigKey
	for _, name := range configName {
		configKeys = append(configKeys, &defangv1.ConfigKey{Project: projectName, Name: name})
	}

	resp, err := provider.GetConfigs(ctx, &defangv1.GetConfigsRequest{Configs: configKeys})
	if err != nil {
		return &defangv1.GetConfigsResponse{}, err
	}

	return resp, nil
}
