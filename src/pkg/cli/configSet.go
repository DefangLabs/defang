package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, client client.Client, name string, value string, isSensitive bool) error {
	project, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Setting config %q in project %q", name, project)

	if DoDryRun {
		return ErrDryRun
	}

	return client.PutConfig(ctx, &defangv1.Config{Project: project, Name: name, Value: value, IsSensitive: isSensitive})
}
