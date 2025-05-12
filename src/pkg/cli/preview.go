package cli

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Preview(ctx context.Context, project *compose.Project, fabric cliClient.FabricClient, provider cliClient.Provider, mode defangv1.DeploymentMode) error {
	resp, project, err := ComposeUp(ctx, project, fabric, provider, compose.UploadModePreview, mode)
	if err != nil {
		return err
	}

	options := TailOptions{Deployment: resp.Etag, LogType: logs.LogTypeBuild, Verbose: true}
	return tailAndWaitForCD(ctx, project.Name, provider, options, LogEntryPrintHandler)
}
