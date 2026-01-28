package command

import (
	"encoding/json"
	"errors"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/auth"
	"github.com/DefangLabs/defang/src/pkg/cli"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/spf13/cobra"
)

func ListWorkspaces(cmd *cobra.Command, args []string) error {
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
	Annotations: authNeededAlways,
	Short:       "Manage workspaces",
	RunE:        ListWorkspaces,
}

var workspaceListCmd = &cobra.Command{
	Use:         "ls",
	Aliases:     []string{"list"},
	Args:        cobra.NoArgs,
	Annotations: authNeededAlways,
	Short:       "List available workspaces",
	RunE:        ListWorkspaces,
}

func init() {
	workspaceCmd.Flags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "print output in JSON format")
	workspaceListCmd.Flags().Bool("json", pkg.GetenvBool("DEFANG_JSON"), "print output in JSON format")
	workspaceCmd.AddCommand(workspaceListCmd)
}
