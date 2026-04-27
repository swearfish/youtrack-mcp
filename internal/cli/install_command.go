package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"youtrack-mcp/internal/envfile"
	"youtrack-mcp/internal/install"
)

type installOptions struct {
	workspace string
	binary    string
	dryRun    bool
}

func newInstallCommand() *cobra.Command {
	options := &installOptions{}

	cmd := &cobra.Command{
		Use:   "install [vscode|copilot]",
		Short: "Register the MCP server in VS Code or Copilot configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]
			envFile, err := cmd.Root().PersistentFlags().GetString("env-file")
			if err != nil {
				return err
			}
			overrideEnv, err := cmd.Root().PersistentFlags().GetBool("override-env")
			if err != nil {
				return err
			}
			if envFile != "" {
				envFile, err = envfile.Resolve(envFile)
				if err != nil {
					return err
				}
			}
			result, err := install.Register(install.RegisterOptions{
				Target:      target,
				Workspace:   options.workspace,
				Binary:      options.binary,
				DryRun:      options.dryRun,
				EnvFile:     envFile,
				OverrideEnv: overrideEnv,
			})
			if err != nil {
				return err
			}

			fmt.Println(result.Message)
			if result.ConfigPath != "" {
				fmt.Printf("Config: %s\n", result.ConfigPath)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&options.workspace, "workspace", "", "Write a workspace-local MCP config under the given directory")
	cmd.Flags().StringVar(&options.binary, "binary", "", "Path to the youtrack-mcp executable to register")
	cmd.Flags().BoolVar(&options.dryRun, "dry-run", false, "Print the config change without writing it")

	return cmd
}
