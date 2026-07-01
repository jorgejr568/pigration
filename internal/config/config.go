// Package config loads and interpolates the pigration `.db-migration.yaml`
// configuration file and assembles a Postgres DSN from it.
package config

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the parsed, interpolated representation of `.db-migration.yaml`.
type Config struct {
	Database struct {
		URL      string `yaml:"url"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"database"`
	Migrations struct {
		Dir     string `yaml:"dir"`
		Package string `yaml:"package"`
		Table   string `yaml:"table"`
	} `yaml:"migrations"`
	Fresh struct {
		Allow bool `yaml:"allow"`
	} `yaml:"fresh"`
}

var interpolateRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?\}`)

// Interpolate expands `${VAR}` and `${VAR:-default}` references in s using the
// provided getenv lookup. `${VAR}` yields the env value (empty if unset).
// `${VAR:-default}` yields the env value, or default when the env var is unset
// or empty. Anything else is left literal.
func Interpolate(s string, getenv func(string) string) string {
	return interpolateRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := interpolateRe.FindStringSubmatch(match)
		name := groups[1]
		hasDefault := groups[2] != ""
		def := groups[3]
		val := getenv(name)
		if val == "" && hasDefault {
			return def
		}
		return val
	})
}

// Load reads the config file at path, interpolates every string value against
// the process environment, and applies defaults. A missing file yields an error
// that points the user at `pigration init`.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file %q not found: run `pigration init` to create it", path)
		}
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}

	interp := func(s string) string { return Interpolate(s, os.Getenv) }
	c.Database.URL = interp(c.Database.URL)
	c.Database.Host = interp(c.Database.Host)
	c.Database.Port = interp(c.Database.Port)
	c.Database.User = interp(c.Database.User)
	c.Database.Password = interp(c.Database.Password)
	c.Database.Name = interp(c.Database.Name)
	c.Database.SSLMode = interp(c.Database.SSLMode)
	c.Migrations.Dir = interp(c.Migrations.Dir)
	c.Migrations.Package = interp(c.Migrations.Package)
	c.Migrations.Table = interp(c.Migrations.Table)

	if c.Migrations.Dir == "" {
		c.Migrations.Dir = "./migrations"
	}
	if c.Migrations.Package == "" {
		c.Migrations.Package = "migrations"
	}
	if c.Migrations.Table == "" {
		c.Migrations.Table = "schema_migrations"
	}
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}

	return &c, nil
}

// DSN returns the Postgres connection string. If database.url is set it wins;
// otherwise a DSN is assembled from the discrete params, requiring host, user,
// and name. DSN is a pure function of the struct: Load owns defaults and the
// URLConflicts warning, so DSN performs no terminal I/O.
func (c *Config) DSN() (string, error) {
	if c.Database.URL != "" {
		return c.Database.URL, nil
	}

	if c.Database.Host == "" {
		return "", fmt.Errorf("database DSN incomplete: database.host is empty (and database.url is unset)")
	}
	if c.Database.User == "" {
		return "", fmt.Errorf("database DSN incomplete: database.user is empty (and database.url is unset)")
	}
	if c.Database.Name == "" {
		return "", fmt.Errorf("database DSN incomplete: database.name is empty (and database.url is unset)")
	}

	host := c.Database.Host
	if c.Database.Port != "" {
		host = host + ":" + c.Database.Port
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   host,
		Path:   "/" + c.Database.Name,
	}
	if c.Database.Password != "" {
		u.User = url.UserPassword(c.Database.User, c.Database.Password)
	} else {
		u.User = url.User(c.Database.User)
	}
	// Load guarantees SSLMode is non-empty; for directly-constructed configs,
	// omit the parameter rather than emit sslmode= (which pgx rejects).
	if c.Database.SSLMode != "" {
		q := u.Query()
		q.Set("sslmode", c.Database.SSLMode)
		u.RawQuery = q.Encode()
	}

	return u.String(), nil
}

// DBName returns the target database name for user-facing confirmation prompts,
// or "" when it cannot be determined. It mirrors DSN's precedence: when
// database.url is set, the name is parsed out of the URL; otherwise the discrete
// database.name is used. The URL path is only trusted for postgres/postgresql
// URLs, so a key/value DSN (host=... dbname=...) yields "" rather than leaking
// the raw string.
func (c *Config) DBName() string {
	if c.Database.URL != "" {
		u, err := url.Parse(c.Database.URL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
			return ""
		}
		return strings.TrimPrefix(u.Path, "/")
	}
	return c.Database.Name
}

// URLConflicts reports whether database.url is set alongside discrete
// database.* fields (which are then ignored). The CLI warns once when this holds.
func (c *Config) URLConflicts() bool {
	return c.Database.URL != "" &&
		(c.Database.Host != "" || c.Database.User != "" || c.Database.Name != "")
}
