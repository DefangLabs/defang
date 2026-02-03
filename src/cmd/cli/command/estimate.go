package command

import (
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
	"github.com/spf13/cobra"
)

func makeEstimateCmd() *cobra.Command {
	var estimateCmd = &cobra.Command{
		Use:         "estimate",
		Args:        cobra.NoArgs,
		Annotations: authNeededAlways,
		Short:       "Estimate the cost of deploying the current project",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			region, _ := cmd.Flags().GetString("region")

			loader := configureLoader(cmd)
			project, err := loader.LoadProject(ctx)
			if err != nil {
				return err
			}

			if global.Stack.Provider == client.ProviderAuto {
				providerID, err := interactiveSelectProvider([]client.ProviderID{
					client.ProviderAWS,
					client.ProviderGCP,
				})
				if err != nil {
					return fmt.Errorf("failed to select provider: %w", err)
				}
				global.Stack.Provider = providerID
			}

			var previewProvider client.Provider = &client.PlaygroundProvider{FabricClient: global.Client}

			// default to development mode if not specified; TODO: when mode is not specified, show an interactive prompt
			if global.Stack.Mode == modes.ModeUnspecified {
				global.Stack.Mode = modes.ModeAffordable
			}
			if region == "" {
				region = client.GetRegion(global.Stack.Provider) // This sets the default region based on the provider
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
	estimateCmd.Flags().StringVarP(&global.Stack.Region, "region", "r", global.Stack.Region, "which cloud region to estimate")
	return estimateCmd
}

var providerDescription = map[client.ProviderID]string{
	client.ProviderDefang: "The Defang Playground is a free platform intended for testing purposes only.",
	client.ProviderAWS:    "Deploy to AWS using the AWS_* environment variables or the AWS CLI configuration.",
	client.ProviderDO:     "Deploy to DigitalOcean using the DIGITALOCEAN_TOKEN, SPACES_ACCESS_KEY_ID, and SPACES_SECRET_ACCESS_KEY environment variables.",
	client.ProviderGCP:    "Deploy to Google Cloud Platform using gcloud Application Default Credentials.",
}

func interactiveSelectProvider(providers []client.ProviderID) (client.ProviderID, error) {
	if len(providers) < 2 {
		panic("interactiveSelectProvider called with less than 2 providers")
	}
	// Prompt the user to choose a provider if in interactive mode
	options := []string{}
	for _, p := range providers {
		options = append(options, p.String())
	}
	// Default to the provider in the environment if available
	var defaultOption any // not string!
	if pkg.AwsInEnv() != "" {
		defaultOption = client.ProviderAWS.String()
	} else if pkg.GcpInEnv() != "" {
		defaultOption = client.ProviderGCP.String()
	}
	var optionValue string
	if err := survey.AskOne(&survey.Select{
		Default: defaultOption,
		Message: "Choose a cloud provider:",
		Options: options,
		Help:    "The provider you choose will be used for deploying services.",
		Description: func(value string, i int) string {
			return providerDescription[client.ProviderID(value)]
		},
	}, &optionValue, survey.WithStdio(term.DefaultTerm.Stdio())); err != nil {
		return "", fmt.Errorf("failed to select provider: %w", err)
	}
	track.Evt("ProviderSelected", P("provider", optionValue))
	var providerID client.ProviderID
	err := providerID.Set(optionValue)
	if err != nil {
		return "", err
	}
	return providerID, nil
}
