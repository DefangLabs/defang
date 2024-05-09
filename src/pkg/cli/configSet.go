package cli

import (
	"context"

	"github.com/defang-io/defang/src/pkg/cli/client"
	"github.com/defang-io/defang/src/pkg/term"
	defangv1 "github.com/defang-io/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, client client.Client, name string, value string) error {
	projectName, err := client.LoadProjectName()
	if err != nil {
		return err
	}
	term.Debug(" - Setting config", name, "in project", projectName)

	if DoDryRun {
		return ErrDryRun
	}

	return client.PutConfig(ctx, &defangv1.SecretValue{Name: name, Value: value})
}
