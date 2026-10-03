package command

import (
	"errors"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/scope"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:         "token",
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "Manage personal access tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		var s, _ = cmd.Flags().GetString("scope")
		var expires, _ = cmd.Flags().GetDuration("expires")
		var save, _ = cmd.Flags().GetBool("save")
		if save && client.UsingAccessTokenEnv() {
			return errors.New("cannot save access token while DEFANG_ACCESS_TOKEN is set; unset it and try again")
		}

		var assertion string
		if global.NonInteractive {
			token := client.GetExistingToken(global.FabricAddr)
			if auth.IsOpenAuthAccessToken(token) {
				assertion = token
			} else {
				term.Debug("Existing credential is not an OpenAuth login; authenticating in the browser")
			}
		}

		token, err := cli.Token(cmd.Context(), global.Client, global.TenantSelection, expires, scope.Scope(s), assertion)
		if err != nil {
			return err
		}

		if save {
			if err := client.SaveAccessToken(global.FabricAddr, token); err != nil {
				return err
			}
			term.Info("Scoped access token saved")
			return nil
		}

		term.Printc(term.BrightCyan, "Scoped access token: ")
		term.Println(token)
		return nil
	},
}
