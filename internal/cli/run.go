package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jorgejr568/pigration/internal/codegen"
	"github.com/jorgejr568/pigration/internal/config"
)

const runnerDir = ".db-migration/runner"

// prepareRunner renders and writes the throwaway `go run` entrypoint. It returns
// the path to the generated main.go. Pure filesystem — no execution.
func prepareRunner(cfg *config.Config) (string, error) {
	importPath, err := codegen.ModuleImportPath("go.mod", cfg.Migrations.Dir)
	if err != nil {
		return "", err
	}
	src, err := codegen.RenderRunner(importPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(runnerDir, 0o755); err != nil {
		return "", fmt.Errorf("creating runner dir: %w", err)
	}
	mainPath := filepath.Join(runnerDir, "main.go")
	if err := os.WriteFile(mainPath, []byte(src), 0o644); err != nil {
		return "", fmt.Errorf("writing runner: %w", err)
	}
	return mainPath, nil
}

// runViaGoRun generates the entrypoint and executes `go run` against it, passing
// configuration through environment variables.
func runViaGoRun(cmd *cobra.Command, cfg *config.Config, pigCmd string, extraEnv map[string]string) error {
	if _, err := prepareRunner(cfg); err != nil {
		return err
	}
	dsn, err := cfg.DSN()
	if err != nil {
		return err
	}

	env := os.Environ()
	env = append(env,
		"PIGRATION_DSN="+dsn,
		"PIGRATION_TABLE="+cfg.Migrations.Table,
		"PIGRATION_CMD="+pigCmd,
	)
	for k, v := range extraEnv {
		if v != "" {
			env = append(env, k+"="+v)
		}
	}

	c := exec.Command("go", "run", "./"+runnerDir)
	c.Env = env
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

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return err
			}
			steps, _ := cmd.Flags().GetInt("steps")
			extra := map[string]string{}
			if steps > 0 {
				extra["PIGRATION_STEPS"] = strconv.Itoa(steps)
			}
			return runViaGoRun(cmd, cfg, "up", extra)
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
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return err
			}
			steps, _ := cmd.Flags().GetInt("steps")
			batch, _ := cmd.Flags().GetInt("batch")
			extra := map[string]string{}
			if steps > 0 {
				extra["PIGRATION_STEPS"] = strconv.Itoa(steps)
			}
			if batch > 0 {
				extra["PIGRATION_BATCH"] = strconv.Itoa(batch)
			}
			return runViaGoRun(cmd, cfg, "down", extra)
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
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return err
			}
			return runViaGoRun(cmd, cfg, "status", nil)
		},
	}
}

func newFreshCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fresh",
		Short: "DESTRUCTIVE: drop the public schema and re-apply all migrations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath(cmd))
			if err != nil {
				return err
			}

			allowed := cfg.Fresh.Allow || os.Getenv("PIGRATION_ALLOW_FRESH") == "1"
			if !allowed {
				return fmt.Errorf("fresh refused: set fresh.allow: true in config or PIGRATION_ALLOW_FRESH=1")
			}

			force, _ := cmd.Flags().GetBool("force")
			if !force {
				dbName := targetDBName(cfg)
				if err := confirmFresh(cmd, dbName); err != nil {
					return err
				}
			}

			return runViaGoRun(cmd, cfg, "fresh", map[string]string{
				"PIGRATION_ALLOW_FRESH": "1",
			})
		},
	}
	cmd.Flags().Bool("force", false, "skip the interactive confirmation prompt")
	return cmd
}

// targetDBName extracts the database name from the config for the confirmation
// prompt, falling back to a placeholder when it cannot be determined.
func targetDBName(cfg *config.Config) string {
	if cfg.Database.Name != "" {
		return cfg.Database.Name
	}
	if dsn, err := cfg.DSN(); err == nil {
		if i := strings.LastIndex(dsn, "/"); i >= 0 {
			name := dsn[i+1:]
			if j := strings.IndexAny(name, "?"); j >= 0 {
				name = name[:j]
			}
			if name != "" {
				return name
			}
		}
	}
	return "the target database"
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
