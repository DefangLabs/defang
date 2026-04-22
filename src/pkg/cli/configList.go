package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type PrintConfig struct {
	Name string
}

func ConfigList(ctx context.Context, projectName string, provider client.Provider) error {
	slog.Debug(fmt.Sprintf("Listing config in project %q", projectName))

	config, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
	if err != nil {
		return err
	}

	numConfigs := len(config.Names)
	if numConfigs == 0 {
		slog.WarnContext(ctx, "No configs found")
		return nil
	}

	configNames := make([]PrintConfig, numConfigs)
	for i, c := range config.Names {
		configNames[i] = PrintConfig{Name: c}
	}

	return term.Table(configNames, "Name")
}
