package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, client client.Client, name string, value string, attributes *[]defangv1.ConfigAttributes) error {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return err
	}
	term.Debugf("Setting config %q in project %q", name, projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return client.PutConfig(ctx, &defangv1.PutValue{Name: name, Value: value, Attributes: *attributes})
}
