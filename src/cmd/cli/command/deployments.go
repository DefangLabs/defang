package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

var deploymentsCmd = &cobra.Command{
	Use:         "deployments",
	Aliases:     []string{"deployment", "deploys", "deps", "dep"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List all active deployments",
	RunE: func(cmd *cobra.Command, args []string) error {
		return deploymentsList(cmd, defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE)
	},
}

var deploymentsListCmd = &cobra.Command{
	Use:         "history",
	Aliases:     []string{"ls", "list"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List deployment history for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		return deploymentsList(cmd, defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY)
	},
}

func deploymentsList(cmd *cobra.Command, listType defangv1.DeploymentType) error {
	var utc, _ = cmd.Flags().GetBool("utc")
	var limit, _ = cmd.Flags().GetUint32("limit")

	if utc {
		cli.EnableUTCMode()
	}

	loader := configureLoader(cmd)
	projectName, err := loader.LoadProjectName(cmd.Context())
	if err != nil {
		return err
	}

	return cli.DeploymentsList(cmd.Context(), global.Client, cli.ListDeploymentsParams{
		ListType:    listType,
		ProjectName: projectName,
		StackName:   global.Stack.Name,
		Limit:       limit,
	})
}
