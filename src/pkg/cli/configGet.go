package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func ConfigGet(ctx context.Context, client client.Client, names ...string) (*defangv1.ConfigValues, error) {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return nil, err
	}
	term.Debugf("get config %v in project %q", names, projectName)

	if DoDryRun {
		return nil, ErrDryRun
	}

	config, err := client.GetConfig(ctx, &defangv1.Configs{Names: names, Project: projectName})
	if err != nil {
		var e *types.InvalidParameters
		if errors.As(err, &e) {
			term.Warnf("Config %v not found in project %q", names, projectName)
			return config, nil
		}

		return nil, err
	}

	PrintConfigData(config)

	return config, nil
}
