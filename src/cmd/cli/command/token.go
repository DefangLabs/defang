package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/scope"
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

		return cli.Token(cmd.Context(), global.Client, global.Tenant, expires, scope.Scope(s))
	},
}
