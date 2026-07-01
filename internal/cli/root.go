// Package cli implements the pigration Cobra command tree.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/jorgejr568/pigration/internal/config"
)

// DefaultConfigPath is the default location of the config file.
const DefaultConfigPath = "./.db-migration.yaml"

// newRootCmd builds the root command with all subcommands wired in.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "pigration",
		Short:         "pigration — Go + Postgres migrations as self-registering code",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("config", DefaultConfigPath, "path to .db-migration.yaml")

	root.AddCommand(newInitCmd(), newMakeCmd())
	root.AddCommand(newRunCmd()...)
	return root
}

// configPath extracts the --config persistent flag value.
func configPath(cmd *cobra.Command) string {
	p, err := cmd.Flags().GetString("config")
	if err != nil || p == "" {
		return DefaultConfigPath
	}
	return p
}

// loadConfig loads the config for cmd, resolving the --config flag. The error is
// returned unwrapped so CLI output matches config.Load exactly.
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	return config.Load(configPath(cmd))
}

// Execute runs the pigration CLI.
func Execute() error {
	return newRootCmd().Execute()
}
