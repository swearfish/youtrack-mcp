package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	internalmcp "youtrack-mcp/internal/mcp"
)

func newMCPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the YouTrack MCP server on stdio",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return internalmcp.Run(ctx)
		},
	}
}
