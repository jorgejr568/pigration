package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jorgejr568/pigration/internal/codegen"
	"github.com/jorgejr568/pigration/internal/config"
)

const runnerDir = ".db-migration/runner"

// prepareRunner renders and writes the throwaway `go run` entrypoint to
// runnerDir/main.go. Pure filesystem — no execution.
func prepareRunner(cfg *config.Config) error {
	importPath, err := codegen.ModuleImportPath("go.mod", cfg.Migrations.Dir)
	if err != nil {
		return err
	}
	src, err := codegen.RenderRunner(importPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		return fmt.Errorf("creating runner dir: %w", err)
	}
	mainPath := filepath.Join(runnerDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		return fmt.Errorf("writing runner: %w", err)
	}
	return nil
}

// runViaGoRun generates the entrypoint and executes `go run` against it, passing
// configuration through the PIGRATION_* env protocol. It is the single funnel
// every DB-touching command passes through exactly once, so the url-vs-discrete
// conflict warning is emitted here.
func runViaGoRun(cmd *cobra.Command, cfg *config.Config, renv codegen.RunnerEnv) error {
	if err := prepareRunner(cfg); err != nil {
		return err
	}
	if cfg.URLConflicts() {
		fmt.Fprintln(cmd.ErrOrStderr(), "pigration: database.url is set; ignoring discrete database.* fields")
	}
	dsn, err := cfg.DSN()
	if err != nil {
		return err
	}

	c := exec.Command("go", "run", "./"+runnerDir)
	c.Env = renv.Environ(os.Environ(), dsn, cfg.Migrations.Table)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = os.Stdin
	if err := c.Run(); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}
	return nil
}

// newRunCmd returns the DB-touching subcommands.
func newRunCmd() []*cobra.Command {
	return []*cobra.Command{
		newMigrateCmd(),
		newRollbackCmd(),
		newStatusCmd(),
		newFreshCmd(),
	}
}

// positiveFlag reads an int flag that is only meaningful when >= 1. An unset
// flag returns 0 (the "use default behavior" sentinel); an explicitly-set
// non-positive value is refused loudly, because silently falling back to the
// default could e.g. roll back the latest batch when the user asked for a
// specific one.
func positiveFlag(cmd *cobra.Command, name string) (int, error) {
	v, _ := cmd.Flags().GetInt(name)
	if cmd.Flags().Changed(name) && v < 1 {
		return 0, fmt.Errorf("--%s must be >= 1, got %d", name, v)
	}
	return v, nil
}

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			steps, err := positiveFlag(cmd, "steps")
			if err != nil {
				return err
			}
			return runViaGoRun(cmd, cfg, codegen.RunnerEnv{Cmd: "up", Steps: steps})
		},
	}
	cmd.Flags().Int("steps", 0, "apply only the next N migrations")
	return cmd
}

func newRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll back the most recent batch (or --steps N / --batch K)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			steps, err := positiveFlag(cmd, "steps")
			if err != nil {
				return err
			}
			batch, err := positiveFlag(cmd, "batch")
			if err != nil {
				return err
			}
			return runViaGoRun(cmd, cfg, codegen.RunnerEnv{Cmd: "down", Steps: steps, Batch: batch})
		},
	}
	cmd.Flags().Int("steps", 0, "roll back the last N migrations")
	cmd.Flags().Int("batch", 0, "roll back a specific batch number")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show applied and pending migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			return runViaGoRun(cmd, cfg, codegen.RunnerEnv{Cmd: "status"})
		},
	}
}

func newFreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fresh",
		Short: "DESTRUCTIVE: drop the public schema and re-apply all migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(cmd)
			if err != nil {
				return err
			}

			allowed := cfg.Fresh.Allow || os.Getenv(codegen.EnvAllowFresh) == "1"
			if !allowed {
				return fmt.Errorf("fresh refused: set fresh.allow: true in config or %s=1", codegen.EnvAllowFresh)
			}

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				dbName := cfg.DBName()
				if dbName == "" {
					return fmt.Errorf("cannot determine database name for confirmation; re-run with --force")
				}
				if err := confirmFresh(cmd, dbName); err != nil {
					return err
				}
			}

			return runViaGoRun(cmd, cfg, codegen.RunnerEnv{Cmd: "fresh", AllowFresh: true})
		},
	}
	cmd.Flags().Bool("force", false, "skip the interactive confirmation prompt")
	return cmd
}

// confirmFresh prompts the user to type the database name to proceed.
func confirmFresh(cmd *cobra.Command, dbName string) error {
	fmt.Fprintf(cmd.OutOrStdout(),
		"This will DROP ALL data in schema \"public\". Type the database name (%s) to continue: ", dbName)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) != dbName {
		return fmt.Errorf("confirmation did not match %q; aborting", dbName)
	}
	return nil
}
