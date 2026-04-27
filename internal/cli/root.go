package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"youtrack-mcp/internal/envfile"
	"youtrack-mcp/internal/version"
)

type rootOptions struct {
	envFile     string
	overrideEnv bool
}

func NewRootCommand() *cobra.Command {
	options := &rootOptions{}

	cmd := &cobra.Command{
		Use:           "youtrack-mcp",
		Short:         "Standalone YouTrack MCP server and installer",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version.Value,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if options.envFile != "" {
				if _, err := envfile.Apply(options.envFile, envfile.ApplyOptions{Override: options.overrideEnv}); err != nil {
					return err
				}
				return nil
			}

			if _, err := envfile.ApplyDefaultIfPresent(envfile.ApplyOptions{Override: options.overrideEnv}); err != nil {
				return fmt.Errorf("load default .env: %w", err)
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&options.envFile, "env-file", "", "Load environment variables from a dotenv file")
	cmd.PersistentFlags().BoolVar(&options.overrideEnv, "override-env", false, "Allow env-file values to override existing process environment variables")
	cmd.AddCommand(newMCPCommand())
	cmd.AddCommand(newInstallCommand())

	return cmd
}
