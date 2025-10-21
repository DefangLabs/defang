package cli

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/modes"
)

func Preview(ctx context.Context, project *compose.Project, fabric cliClient.FabricClient, provider cliClient.Provider, mode modes.Mode) error {
	resp, project, err := ComposeUp(ctx, fabric, provider, ComposeUpParams{
		Project:    project,
		UploadMode: compose.UploadModePreview,
		Mode:       mode,
	})
	if err != nil {
		return err
	}

	options := TailOptions{Deployment: resp.Etag, LogType: logs.LogTypeBuild, Verbose: true}
	return TailAndWaitForCD(ctx, provider, project.Name, options)
}
