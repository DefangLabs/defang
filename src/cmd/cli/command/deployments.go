package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

var deploymentsCmd = &cobra.Command{
	Use:         "deployments",
	Aliases:     []string{"deployment", "deploys", "deps", "dep", "ls", "list"},
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	Short:       "List all active deployments",
	RunE: func(cmd *cobra.Command, args []string) error {
		var utc, _ = cmd.Flags().GetBool("utc")
		var limit, _ = cmd.Flags().GetUint32("limit")
		var all, _ = cmd.Flags().GetBool("all")
		var listType defangv1.DeploymentType
		if all {
			listType = defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY
		} else {
			listType = defangv1.DeploymentType_DEPLOYMENT_TYPE_ACTIVE
		}

		if utc {
			cli.EnableUTCMode()
		}

		loader := configureLoader(cmd)
		projectName, _, err := loader.LoadProjectName(cmd.Context())
		if err != nil {
			if listType == defangv1.DeploymentType_DEPLOYMENT_TYPE_HISTORY {
				return err
			}
		}

		return cli.DeploymentsList(cmd.Context(), global.Client, cli.ListDeploymentsParams{
			ListType:    listType,
			ProjectName: projectName,
			StackName:   global.Stack.Name,
			Limit:       limit,
		})

	},
}
