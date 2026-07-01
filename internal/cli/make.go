package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jorgejr568/pigration/internal/codegen"
	"github.com/jorgejr568/pigration/internal/config"
)

func newMakeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "make <name>",
		Short: "Scaffold a timestamped migration file",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return err
			}
			name := strings.Join(args, " ")
			return runMake(cmd, cfg, name, time.Now().Unix())
		},
	}
}

func runMake(cmd *cobra.Command, cfg *config.Config, name string, ts int64) error {
	fname, content, err := codegen.RenderMigration(cfg.Migrations.Package, name, ts)
	if err != nil {
		return err
	}
	dir := cfg.Migrations.Dir
	if dir == "" {
		dir = "./migrations"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating migrations dir: %w", err)
	}
	dest := filepath.Join(dir, fname)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("migration %s already exists", dest)
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing migration: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", dest)
	return nil
}
