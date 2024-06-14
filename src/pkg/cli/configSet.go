package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, client client.Client, name string, value string) error {
	project, err := client.LoadProject(ctx)
	if err != nil {
		return err
	}
	term.Debug("Setting config", name, "in project", project.Name)

	if DoDryRun {
		return ErrDryRun
	}

	return client.PutConfig(ctx, &defangv1.SecretValue{Name: name, Value: value})
}
