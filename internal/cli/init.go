package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const defaultConfigTemplate = `database:
  url: ${DATABASE_URL}          # ${VAR} reads env; ${VAR:-default} gives a fallback

  # Discrete-params alternative (comment out url above and use these):
  # host: ${DB_HOST:-localhost}
  # port: 5432
  # user: ${DB_USER}
  # password: ${DB_PASSWORD}
  # name: myapp
  # sslmode: disable

migrations:
  dir: ./migrations
  package: migrations           # package name used in generated files
  table: schema_migrations      # tracking table name

fresh:
  allow: false                  # must be true (or PIGRATION_ALLOW_FRESH=1) for ` + "`fresh`" + `
`

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold .db-migration.yaml, the migrations dir, and .gitignore",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			force, _ := cmd.Flags().GetBool("force")
			cfgPath := configPath(cmd)
			return runInit(cmd, cfgPath, force)
		},
	}
	cmd.Flags().Bool("force", false, "overwrite an existing config file")
	return cmd
}

func runInit(cmd *cobra.Command, cfgPath string, force bool) error {
	out := cmd.OutOrStdout()

	// Config file
	if _, err := os.Stat(cfgPath); err == nil && !force {
		fmt.Fprintf(out, "config %s already exists; leaving it untouched\n", cfgPath)
	} else {
		if dir := filepath.Dir(cfgPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating config dir: %w", err)
			}
		}
		if err := os.WriteFile(cfgPath, []byte(defaultConfigTemplate), 0o644); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
		fmt.Fprintf(out, "wrote %s\n", cfgPath)
	}

	// migrations dir
	if err := os.MkdirAll("migrations", 0o755); err != nil {
		return fmt.Errorf("creating migrations dir: %w", err)
	}
	fmt.Fprintln(out, "ensured migrations/ directory")

	// .gitignore
	if err := ensureGitignore("/.db-migration/"); err != nil {
		return err
	}
	fmt.Fprintln(out, "ensured /.db-migration/ in .gitignore")

	return nil
}

// ensureGitignore appends entry to .gitignore if it is not already present.
func ensureGitignore(entry string) error {
	const name = ".gitignore"
	data, err := os.ReadFile(name)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading .gitignore: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()
	prefix := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		prefix = "\n"
	}
	if _, err := f.WriteString(prefix + entry + "\n"); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}
	return nil
}
