package command

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
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

			if config.ProviderID == cliClient.ProviderAuto {
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
			if config.Mode == modes.ModeUnspecified {
				config.Mode = modes.ModeAffordable
			}
			if region == "" {
				region = cliClient.GetRegion(config.ProviderID) // This sets the default region based on the provider
			}

			estimate, err := cli.RunEstimate(ctx, project, client, previewProvider, config.ProviderID, region, config.Mode)
			if err != nil {
				return fmt.Errorf("failed to run estimate: %w", err)
			}
			term.Debugf("Estimate: %+v", estimate)

			cli.PrintEstimate(config.Mode, estimate, term.DefaultTerm)

			return nil
		},
	}

	estimateCmd.Flags().VarP(&config.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	estimateCmd.Flags().StringP("region", "r", "", "which cloud region to estimate")
	return estimateCmd
}
