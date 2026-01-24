package command

import (
	"encoding/json"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/stacks"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

var whoamiCmd = &cobra.Command{
	Use:         "whoami",
	Args:        cobra.NoArgs,
	Short:       "Show the current user",
	Annotations: authNeededAnnotation,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		jsonMode, _ := cmd.Flags().GetBool("json")

		global.NonInteractive = true // don't show provider prompt

		var provider client.Provider
		session, err := newCommandSessionWithOpts(cmd, commandSessionOpts{
			CheckAccountInfo: false,
		})
		if err != nil {
			if !errors.Is(err, stacks.ErrDefaultStackNotSet) {
				return err
			}
		} else {
			provider = session.Provider
		}

		token := client.GetExistingToken(global.Cluster)

		userInfo, err := auth.FetchUserInfo(ctx, token)
		if err != nil {
			// Either the auth service is down, or we're using a Fabric JWT: skip workspace information
			if !jsonMode {
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
		}

		if jsonMode {
			bytes, err := json.Marshal(data)
			if err != nil {
				return err
			}
			_, err = term.Println(string(bytes))
			return err
		} else {
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
			return term.Table([]cli.ShowAccountData{data}, cols...)
		}
	},
}
