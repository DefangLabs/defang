package command

import (
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
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
			project, _, err := loader.LoadProject(ctx)
			if err != nil {
				return err
			}

			previewProvider, err := newProvider(cmd.Context(), loader)
			if err != nil {
				return err
			}

			err = canIUseProvider(cmd.Context(), previewProvider, project.Name)
			if err != nil {
				return err
			}

			// default to development mode if not specified; TODO: when mode is not specified, show an interactive prompt
			if mode.Value() == defangv1.DeploymentMode_MODE_UNSPECIFIED {
				mode = Mode(defangv1.DeploymentMode_DEVELOPMENT)
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

	estimateCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", allModes()))
	estimateCmd.Flags().StringP("region", "r", pkg.Getenv("AWS_REGION", "us-west-2"), "which cloud region to estimate")
	return estimateCmd
}
