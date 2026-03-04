package command

import (
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:         "whoami",
	Args:        cobra.NoArgs,
	Short:       "Show the current user",
	Annotations: authNeededAlways,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		global.NonInteractive = true // don't show provider prompt

		var provider client.Provider
		session, err := newCommandSessionWithOpts(cmd, commandSessionOpts{
			CheckAccountInfo: false, // because we do it inside cli.Whoami
		})
		if err != nil {
			term.Warnf("Provider account information not available: %v", err)
		} else {
			provider = session.Provider
		}

		token := client.GetExistingToken(global.FabricAddr)

		var userInfo *auth.UserInfo
		// Skip userinfo fetch in non-interactive mode (CI environments)
		if global.HasTty {
			userInfo, err = auth.FetchUserInfo(ctx, token)
			if err != nil {
				// Either the auth service is down, or we're using a Fabric JWT: skip workspace information
				term.Warn("Workspace information unavailable:", err)
			}
		}

		data, err := cli.Whoami(ctx, global.Client, provider, userInfo, global.Tenant)
		if err != nil {
			return err
		}

		if !global.Verbose {
			data.Tenant = ""
			data.TenantID = ""
			if data.SubscriberTier == defangv1.SubscriptionTier_SUBSCRIPTION_TIER_UNSPECIFIED {
				data.SubscriberTier = defangv1.SubscriptionTier_HOBBY // don't show "SUBSCRIPTION_TIER_UNSPECIFIED"
			}
		}

		cols := []string{
			"Workspace",
			"SubscriberTier",
			"Name",
			"Email",
			"Provider",
			"Region",
		}
		if global.Verbose {
			cols = append(cols, "Tenant", "TenantID")
		}
		return term.Table(data, cols...)
	},
}
