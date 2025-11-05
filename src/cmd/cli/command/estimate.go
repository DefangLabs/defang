package command

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

func makeEstimateCmd() *cobra.Command {
	var estimateCmd = &cobra.Command{
		Use:         "estimate",
		Args:        cobra.NoArgs,
		Annotations: authNeededAnnotation,
		Short:       "Estimate the cost of deploying the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			region, _ := cmd.Flags().GetString("region")

			loader := configureLoader(cmd)
			project, err := loader.LoadProject(ctx)
			if err != nil {
				return err
			}

			if providerID == cliClient.ProviderAuto {
				_, err = interactiveSelectProvider([]cliClient.ProviderID{
					cliClient.ProviderAWS,
					cliClient.ProviderGCP,
				})
				if err != nil {
					return fmt.Errorf("failed to select provider: %w", err)
				}
			}

			var previewProvider cliClient.Provider = &cliClient.PlaygroundProvider{FabricClient: client}

			// default to development mode if not specified; TODO: when mode is not specified, show an interactive prompt
			if mode.Value() == defangv1.DeploymentMode_MODE_UNSPECIFIED {
				mode = modes.Mode(defangv1.DeploymentMode_DEVELOPMENT)
			}
			if region == "" {
				region = cliClient.GetRegion(providerID) // This sets the default region based on the provider
			}

			estimate, err := cli.RunEstimate(ctx, project, client, previewProvider, providerID, region, mode.Value())
			if err != nil {
				return fmt.Errorf("failed to run estimate: %w", err)
			}
			term.Debugf("Estimate: %+v", estimate)

			cli.PrintEstimate(mode.Value(), estimate)

			return nil
		},
	}

	estimateCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	estimateCmd.Flags().StringP("region", "r", "", "which cloud region to estimate")
	return estimateCmd
}
