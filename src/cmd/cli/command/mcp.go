package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:         "mcp",
	Short:       "Manage Deployments",
	Annotations: authNeededAnnotation,
}

var mcpServerCmd = &cobra.Command{
	Use:         "serve",
	Short:       "Manage MCP Server",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: Implement MCP server")

		return nil
	},
}

var mcpSetupCmd = &cobra.Command{
	Use:         "setup",
	Short:       "Manage MCP Server",
	Annotations: authNeededAnnotation,
	Args:        cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("TODO: Implement MCP setup")

		return nil
	},
}
