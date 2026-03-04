package command

import (
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Args:  cobra.NoArgs,
	Short: "Authenticate to Defang",
	RunE: func(cmd *cobra.Command, args []string) error {
		trainingOptOut, _ := cmd.Flags().GetBool("training-opt-out")

		if global.NonInteractive {
			if err := login.NonInteractiveGitHubLogin(cmd.Context(), global.Client, global.FabricAddr); err != nil {
				return err
			}
		} else {
			err := login.InteractiveLogin(cmd.Context(), global.FabricAddr)
			if err != nil {
				return err
			}

			printDefangHint("To generate a sample service, do:", "generate")
		}

		if trainingOptOut {
			req := &defangv1.SetOptionsRequest{TrainingOptOut: trainingOptOut}
			if err := global.Client.SetOptions(cmd.Context(), req); err != nil {
				return err
			}
			term.Info("Options updated successfully")
		}
		return nil
	},
}
