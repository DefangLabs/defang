package cli

import (
	"context"

	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	"github.com/DefangLabs/defang/src/pkg/logs"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func Preview(ctx context.Context, project *compose.Project, fabric cliClient.FabricClient, provider cliClient.Provider) error {
	resp, project, err := ComposeUp(ctx, project, fabric, provider, compose.UploadModePreview, defangv1.DeploymentMode_MODE_UNSPECIFIED)
	if err != nil {
		return err
	}

	return tailAndWaitForCD(ctx, project.Name, provider, TailOptions{Deployment: resp.Etag, LogType: logs.LogTypeBuild, Verbose: true})
}
