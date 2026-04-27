package cli

import (
	"context"

	"github.com/spf13/cobra"

	internalmcp "youtrack-mcp/internal/mcp"
)

func newMCPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the YouTrack MCP server on stdio",
		RunE: func(_ *cobra.Command, _ []string) error {
			return internalmcp.Run(context.Background())
		},
	}
}
