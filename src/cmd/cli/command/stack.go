package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	cliClient "github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/modes"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

func makeStackCmd() *cobra.Command {
	var stackCmd = &cobra.Command{
		Use:   "stack",
		Args:  cobra.NoArgs,
		Short: "Manage Defang deployment stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	stackNewCmd := makeStackNewCmd()
	stackCmd.AddCommand(stackNewCmd)
	stackListCmd := makeStackListCmd()
	stackCmd.AddCommand(stackListCmd)
	stackRemoveCmd := makeStackRemoveCmd()
	stackRemoveCmd.Hidden = true
	stackCmd.AddCommand(stackRemoveCmd)
	return stackCmd
}

func makeStackNewCmd() *cobra.Command {
	var stackNewCmd = &cobra.Command{
		Use:     "new STACK_NAME",
		Aliases: []string{"init"},
		Args:    cobra.MaximumNArgs(1),
		Short:   "Create a new Defang deployment stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			var stackName string
			if len(args) > 0 {
				stackName = args[0]
			}

			var region, _ = cmd.Flags().GetString("region")

			params := stacks.StackParameters{
				Name:     stackName,
				Provider: providerID, // default provider
				Region:   region,
				Mode:     mode,
			}

			if nonInteractive {
				_, err := stacks.Create(params)
				return err
			}

			if params.Provider == cliClient.ProviderAuto {
				var provider string

				err := survey.AskOne(&survey.Select{
					Message: "Which cloud provider do you want to deploy to?",
					Options: []string{"AWS", "GCP"},
					Default: "AWS",
				}, &provider, survey.WithStdio(term.DefaultTerm.Stdio()))
				if err != nil {
					return err
				}

				if provider == "" {
					return errors.New("a cloud provider must be selected")
				}

				err = providerID.Set(provider)
				if err != nil {
					return err
				}
				params.Provider = providerID
			}

			if params.Region == "" {
				defaultRegion := ""
				switch providerID {
				case cliClient.ProviderAWS:
					defaultRegion = "us-west-2"
				case cliClient.ProviderGCP:
					defaultRegion = "us-central1"
				}

				var region string

				err := survey.AskOne(&survey.Input{
					Message: fmt.Sprintf("Which %s region do you want to deploy to?", strings.ToUpper(params.Provider.String())),
					Default: defaultRegion,
				}, &region, survey.WithStdio(term.DefaultTerm.Stdio()))
				if err != nil {
					return err
				}

				params.Region = region
			}

			if params.Mode == modes.ModeUnspecified {
				var selectedMode string
				err := survey.AskOne(&survey.Select{
					Message: "Which deployment mode do you want to use?",
					Help:    "Learn about the different deployment modes at https://docs.defang.io/docs/concepts/deployment-modes",
					Options: modes.AllDeploymentModes(),
					Default: modes.ModeBalanced.String(),
				},
					&selectedMode, survey.WithStdio(term.DefaultTerm.Stdio()))
				if err != nil {
					return err
				}

				modeParsed, err := modes.Parse(selectedMode)
				if err != nil {
					return err
				}
				params.Mode = modeParsed
			}

			if stackName == "" {
				defaultName := fmt.Sprintf("%s-%s", strings.ToLower(params.Provider.String()), params.Region)
				var name string
				err := survey.AskOne(&survey.Input{
					Message: "What do you want to call this stack?",
					Default: defaultName,
				}, &name, survey.WithStdio(term.DefaultTerm.Stdio()))
				if err != nil {
					return err
				}

				params.Name = name
			}

			term.Debugf("Creating stack with parameters: %+v\n", params)

			filename, err := stacks.Create(params)
			if err != nil {
				return err
			}

			term.Infof(
				"Created new stack configuration file: `%s`. "+
					"Check this file into version control. "+
					"You can now deploy this stack using `defang up --stack=%s`\n",
				filename, params.Name,
			)
			return nil
		},
	}
	stackNewCmd.Flags().VarP(&mode, "mode", "m", fmt.Sprintf("deployment mode; one of %v", modes.AllDeploymentModes()))
	stackNewCmd.Flags().StringP("region", "r", "", "Cloud region for the stack deployment")

	return stackNewCmd
}

func makeStackListCmd() *cobra.Command {
	stackListCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		Short:   "List existing Defang deployment stacks",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonMode, _ := cmd.Flags().GetBool("json")

			stacks, err := stacks.List()
			if err != nil {
				return err
			}

			if jsonMode {
				jsonData := []byte("[]")
				if len(stacks) > 0 {
					jsonData, err = json.MarshalIndent(stacks, "", "  ")
					if err != nil {
						return err
					}
				}

				_, err = term.Print(string(jsonData) + "\n")
				return err
			}

			if len(stacks) == 0 {
				_, err = term.Infof("No Defang stacks found in the current directory.\n")
				return err
			}

			return term.Table(stacks, "Name", "Provider", "Region", "Mode")
		},
	}
	stackListCmd.Flags().Bool("json", false, "Output in JSON format")
	return stackListCmd
}

func makeStackRemoveCmd() *cobra.Command {
	var stackRemoveCmd = &cobra.Command{
		Use:     "remove STACK_NAME",
		Aliases: []string{"rm"},
		Args:    cobra.ExactArgs(1),
		Short:   "Remove an existing Defang deployment stack",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			return stacks.Remove(name)
		},
	}
	return stackRemoveCmd
}
