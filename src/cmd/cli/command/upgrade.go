package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/spf13/cobra"
)

var upgradeCmd = &cobra.Command{
	Use:     "upgrade",
	Args:    cobra.NoArgs,
	Aliases: []string{"update"},
	Short:   "Upgrade the Defang CLI to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		global.HideUpdate = true
		return cli.Upgrade(cmd.Context())
	},
}
