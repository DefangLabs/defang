package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, loader compose.Loader, provider client.Provider, name string, value string) error {
	projectName, err := LoadProjectName(ctx, loader, provider)
	if err != nil {
		return err
	}
	term.Debugf("Setting config %q in project %q", name, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return provider.PutConfig(ctx, &defangv1.PutConfigRequest{Name: name, Value: value})
}
