package cli

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
)

func Preview(ctx context.Context, project *compose.Project, fabric cliClient.FabricClient, provider cliClient.Provider, params ComposeUpParams) error {
	resp, project, err := ComposeUp(ctx, fabric, provider, params)
	if err != nil {
		return err
	}

	options := TailOptions{Deployment: resp.Etag, LogType: logs.LogTypeBuild, Verbose: true}
	return TailAndWaitForCD(ctx, provider, project.Name, options)
}
