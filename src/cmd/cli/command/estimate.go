package command

import (
	"fmt"
	"strings"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc/aws"
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

			var previewProvider cliClient.Provider
			if awsInEnv() && providerID == cliClient.ProviderAWS {
				previewProvider = aws.NewByocProvider(ctx, client.GetTenantName())
			} else {
				previewProvider = &cliClient.PlaygroundProvider{FabricClient: client}
			}
			if providerID == cliClient.ProviderAuto || providerID == cliClient.ProviderDefang {
				providerID = cliClient.ProviderAWS
			} else {
				return fmt.Errorf("unsupported provider %s; must be one of %v", providerID, []cliClient.ProviderID{cliClient.ProviderAWS})
			}

			// TODO: bring this back when GCP is supported
			// if providerID == cliClient.ProviderAuto || providerID == cliClient.ProviderDefang {
			// 	if _, err := interactiveSelectProvider([]cliClient.ProviderID{cliClient.ProviderAWS, cliClient.ProviderGCP}); err != nil {
			// 		return err
			// 	}
			// }

			estimate, err := cli.RunEstimate(ctx, project, client, previewProvider, providerID, region, mode.Value())
			if err != nil {
				return fmt.Errorf("failed to run estimate: %w", err)
			}
			term.Debugf("Estimate: %+v", estimate)

			cli.PrintEstimate(mode.Value(), estimate)

			return nil
		},
	}

	estimateCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", strings.Join(allModes(), ", ")))
	estimateCmd.Flags().StringP("region", "r", pkg.Getenv("AWS_REGION", "us-west-2"), "which cloud region to estimate")
	return estimateCmd
}
