package cli

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func ConfigSet(ctx context.Context, insensitive bool, projectName string, provider client.Provider, name string, value string) error {
	term.Debugf("Setting config %q in project %q", name, projectName)

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	req := defangv1.PutConfigRequest{Project: projectName, Name: name, Value: value}
	if insensitive {
		req.Type = defangv1.ConfigType_CONFIGTYPE_INSENSITIVE
	}

	return provider.PutConfig(ctx, &req)
}
