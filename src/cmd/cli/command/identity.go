package command

import (
	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/spf13/cobra"
)

var identityCmd = &cobra.Command{
	Use:   "identity",
	Args:  cobra.NoArgs,
	Short: "Manage the keys your id.defang.io issuer publishes for cloud federation",
}

func identityTenant(cmd *cobra.Command) string {
	if tenant, _ := cmd.Flags().GetString("tenant"); tenant != "" {
		return tenant
	}
	return pkg.Getenv("DEFANG_ID_TENANT", "")
}

var identityClaimCmd = &cobra.Command{
	Use:         "claim TENANT",
	Annotations: authNeededAlways,
	Args:        cobra.ExactArgs(1),
	Short:       "Claim a tenant name; its issuer is https://TENANT.id.defang.io",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.IdentityClaim(cmd.Context(), client.GetExistingToken(global.FabricAddr), args[0])
	},
}

var identityRegisterCmd = &cobra.Command{
	Use:         "register",
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "Enable this machine's public key in your tenant's issuer",
	RunE: func(cmd *cobra.Command, args []string) error {
		ttl, _ := cmd.Flags().GetDuration("ttl")
		label, _ := cmd.Flags().GetString("label")
		return cli.IdentityRegister(cmd.Context(), client.GetExistingToken(global.FabricAddr), identityTenant(cmd), label, ttl)
	},
}

var identityListCmd = &cobra.Command{
	Use:         "list",
	Aliases:     []string{"ls"},
	Annotations: authNeededAlways,
	Args:        cobra.NoArgs,
	Short:       "List the keys enabled in your tenant's issuer",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.IdentityList(cmd.Context(), client.GetExistingToken(global.FabricAddr), identityTenant(cmd))
	},
}

var identityRevokeCmd = &cobra.Command{
	Use:         "revoke KID",
	Annotations: authNeededAlways,
	Args:        cobra.ExactArgs(1),
	Short:       "Remove a key from your tenant's issuer",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cli.IdentityRevoke(cmd.Context(), client.GetExistingToken(global.FabricAddr), identityTenant(cmd), args[0])
	},
}
