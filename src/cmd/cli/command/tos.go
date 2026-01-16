package command

import (
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/spf13/cobra"
)

var tosCmd = &cobra.Command{
	Use:     "terms",
	Aliases: []string{"tos", "eula", "tac", "tou"},
	Args:    cobra.NoArgs,
	Short:   "Read and/or agree the Defang terms of service",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check if we are correctly logged in
		if _, err := global.Client.WhoAmI(cmd.Context()); err != nil {
			return err
		}

		agree, _ := cmd.Flags().GetBool("agree-tos")

		if agree {
			return login.NonInteractiveAgreeToS(cmd.Context(), global.Client)
		}

		if global.NonInteractive {
			printDefangHint("To agree to the terms of service, do:", cmd.CalledAs()+" --agree-tos")
			return nil
		}

		return login.InteractiveAgreeToS(cmd.Context(), global.Client)
	},
}
