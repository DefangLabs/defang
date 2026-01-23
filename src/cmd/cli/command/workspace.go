package command

import (
	"encoding/json"
	"errors"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	"github.com/spf13/cobra"
)

func listWorkspaces(cmd *cobra.Command, args []string) error {
	jsonMode, _ := cmd.Flags().GetBool("json")
	verbose := global.Verbose

	token := client.GetExistingToken(global.Cluster)
	if token == "" {
		return errors.New("no access token found; please log in with `defang login`")
	}

	// Determine current selection from flag/env/token, then reconcile with WhoAmI.
	currentWorkspace := global.Tenant

	info, err := auth.FetchUserInfo(cmd.Context(), token)
	if err != nil {
		return err
	}

	rows := cli.WorkspaceRows(info, currentWorkspace)
	if !verbose {
		for i := range rows {
			rows[i].ID = ""
		}
	}

	if jsonMode {
		out, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		_, err = term.Println(string(out))
		return err
	}

	if len(rows) == 0 {
		term.Info("No workspaces found for this account.")
		return nil
	}

	headers := []string{"Name", "Current"}
	if verbose {
		headers = []string{"Name", "ID", "Current"}
	}

	return term.Table(rows, headers...)
}

var workspaceCmd = &cobra.Command{
	Use:         "workspace",
	Aliases:     []string{"workspaces", "ws"},
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation,
	Short:       "Manage workspaces",
	RunE:        listWorkspaces,
}

var workspaceListCmd = &cobra.Command{
	Use:         "ls",
	Aliases:     []string{"list"},
	Args:        cobra.NoArgs,
	Annotations: authNeededAnnotation,
	Short:       "List available workspaces",
	RunE:        listWorkspaces,
}

var workspaceSelectCmd = &cobra.Command{
	Use:     "select WORKSPACE",
	Aliases: []string{"use", "switch"},
	Args:    cobra.ExactArgs(1),
	// Annotations: authNeededAnnotation,
	Short: "Select a workspace to use",
	RunE: func(cmd *cobra.Command, args []string) error {
		global.Tenant = types.TenantNameOrID(args[0])
		if _, err := cli.ConnectWithTenant(cmd.Context(), global.Cluster, global.Tenant); err != nil {
			return err
		}
		term.Infof("Switched to workspace %q\n", global.Tenant)
		return client.SetCurrentTenant(global.Tenant)
	},
}

func init() {
	workspaceCmd.Flags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "print output in JSON format")
	workspaceListCmd.Flags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "print output in JSON format")
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceSelectCmd)
}
