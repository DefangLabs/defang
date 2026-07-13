package command

import (
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/spf13/cobra"
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Args:  cobra.NoArgs,
	Short: "Manage agent identity keys (public-key registration for cloud federation)",
}

var identityRegisterCmd = &cobra.Command{
	Use:         "register",
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "Register this machine's public key for the current project and stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		ttl, _ := cmd.Flags().GetDuration("ttl")

		// No CheckAccountInfo: registration talks to the key registry, not the cloud provider.
		session, err := newCommandSessionWithOpts(cmd, commandSessionOpts{})
		if err != nil {
			return err
		}
		projectName, err := client.LoadProjectNameWithFallback(cmd.Context(), session.Loader, session.Provider)
		if err != nil {
			return err
		}

		accessToken := client.GetExistingToken(global.FabricAddr)
		return cli.IdentityRegister(cmd.Context(), global.Client, accessToken, projectName, session.Stack.Name, ttl)
	},
}

var identityListCmd = &cobra.Command{
	Use:         "list",
	Aliases:     []string{"ls"},
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "List your registered agent identity keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		accessToken := client.GetExistingToken(global.FabricAddr)
		return cli.IdentityList(cmd.Context(), global.Client, accessToken)
	},
}

var identityRevokeCmd = &cobra.Command{
	Use:         "revoke KID",
	Annotations: authNeededAlways,
	Args:        cobra.ExactArgs(1),
	Short:       "Revoke a registered agent identity key",
	RunE: func(cmd *cobra.Command, args []string) error {
		accessToken := client.GetExistingToken(global.FabricAddr)
		return cli.IdentityRevoke(cmd.Context(), global.Client, accessToken, args[0])
	},
}
