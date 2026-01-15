package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/spf13/cobra"
)

var certCmd = &cobra.Command{
	Use:   "cert",
	Args:  cobra.NoArgs,
	Short: "Manage certificates",
}

var certGenerateCmd = &cobra.Command{
	Use:     "generate",
	Aliases: []string{"gen"},
	Args:    cobra.NoArgs,
	Short:   "Generate a TLS certificate",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		session, err := newCommandSession(cmd)
		if err != nil {
			return err
		}
		project, err := session.Loader.LoadProject(ctx)
		if err != nil {
			return err
		}

		if err := cli.GenerateLetsEncryptCert(ctx, project, global.Client, session.Provider); err != nil {
			return err
		}
		return nil
	},
}
