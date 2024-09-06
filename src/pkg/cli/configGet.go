package cli

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

func ConfigGet(ctx context.Context, client client.Client, names ...string) ([]*defangv1.Config, error) {
	projectName, err := client.LoadProjectName(ctx)
	if err != nil {
		return nil, err
	}
	term.Debugf("get config %v in project %q", names, projectName)

	if DoDryRun {
		return nil, ErrDryRun
	}

	req := defangv1.GetConfigsRequest{}
	for _, name := range names {
		req.Configs = append(req.Configs, &defangv1.ConfigKey{Project: projectName, Name: name})
	}

	resp, err := client.GetConfigs(ctx, &req)
	if err != nil {
		var e *types.InvalidParameters
		if errors.As(err, &e) {
			term.Warnf("Config %v not found in project %q", names, projectName)
			return resp.Configs, nil
		}

		return nil, err
	}

	PrintConfigData(resp.Configs)

	return resp.Configs, nil
}
