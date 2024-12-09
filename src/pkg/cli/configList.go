package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintConfig struct {
	Name string
}

func ConfigList(ctx context.Context, loader client.Loader, provider client.Provider) error {
	projectName, err := client.LoadProjectNameWithFallback(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Listing config in project %q", projectName)

	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
	if err != nil {
		return err
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		_, err := term.Warnf("No configs found")
		return err
	}

	configNames := make([]PrintConfig, numConfigs)
	for i, c := range config.Names {
		configNames[i] = PrintConfig{Name: c}
	}

	return term.Table(configNames, []string{"Name"})
}
