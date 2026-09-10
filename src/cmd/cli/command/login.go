package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/login"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/track"
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

			printActiveWorkspace(cmd)

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

// printActiveWorkspace shows which workspace will be used by default after a successful
// interactive login. The client built before login has no access token yet, so reconnect
// to pick up the one InteractiveLogin just saved.
func printActiveWorkspace(cmd *cobra.Command) {
	ctx := cmd.Context()

	fabric, err := cli.ConnectWithTenant(ctx, global.FabricAddr, global.TenantSelection)
	if err != nil {
		term.Debug("Unable to determine active workspace:", err)
		return
	}
	global.Client = fabric
	track.Tracker = fabric

	data, err := cli.FetchAccountInfo(ctx, fabric, nil, global.FabricAddr, global.TenantSelection, global.HasTty)
	if err != nil {
		term.Debug("Unable to determine active workspace:", err)
		return
	}

	if data.Workspace != "" {
		term.Infof("Using workspace %q\n", data.Workspace)
	}
}
