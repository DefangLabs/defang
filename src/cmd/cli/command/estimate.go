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

			if global.Stack.Provider == cliClient.ProviderAuto {
				_, err = interactiveSelectProvider([]cliClient.ProviderID{
					cliClient.ProviderAWS,
					cliClient.ProviderGCP,
				})
				if err != nil {
					return fmt.Errorf("failed to select provider: %w", err)
				}
			}

			var previewProvider cliClient.Provider = &cliClient.PlaygroundProvider{FabricClient: global.Client}

			// default to development mode if not specified; TODO: when mode is not specified, show an interactive prompt
			if global.Stack.Mode == modes.ModeUnspecified {
				global.Stack.Mode = modes.ModeAffordable
			}
			if region == "" {
				region = cliClient.GetRegion(global.Stack.Provider) // This sets the default region based on the provider
			}

			estimate, err := cli.RunEstimate(ctx, project, global.Client, previewProvider, global.Stack.Provider, region, global.Stack.Mode)
			if err != nil {
				return fmt.Errorf("failed to run estimate: %w", err)
			}
			term.Debugf("Estimate: %+v", estimate)

			cli.PrintEstimate(global.Stack.Mode, estimate, term.DefaultTerm)

			return nil
		},
	}

	estimateCmd.Flags().VarP(&global.Stack.Mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	estimateCmd.Flags().StringVarP(&global.Stack.Region, "region", "r", "", "which cloud region to estimate")
	return estimateCmd
}
