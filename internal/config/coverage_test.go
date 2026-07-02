package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDBName exercises every branch of Config.DBName: URL form with and without
// a path, non-postgres/unparseable URLs (which yield ""), key/value DSNs (which
// are not postgres:// URLs and so also yield ""), and the discrete form.
func TestDBName(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
		want string
	}{
		{
			name: "postgres url with path",
			set:  func(c *Config) { c.Database.URL = "postgres://u:p@h:5432/mydb?sslmode=disable" },
			want: "mydb",
		},
		{
			name: "postgresql scheme with path",
			set:  func(c *Config) { c.Database.URL = "postgresql://u@h/other" },
			want: "other",
		},
		{
			name: "postgres url without path",
			set:  func(c *Config) { c.Database.URL = "postgres://u@h:5432" },
			want: "",
		},
		{
			name: "kv dsn is not a postgres url",
			set:  func(c *Config) { c.Database.URL = "host=localhost user=u dbname=kvdb sslmode=disable" },
			want: "",
		},
		{
			name: "non-postgres scheme yields empty",
			set:  func(c *Config) { c.Database.URL = "mysql://u@h/nope" },
			want: "",
		},
		{
			name: "unparseable url yields empty",
			// A control character in the host makes url.Parse fail.
			set:  func(c *Config) { c.Database.URL = "postgres://ho\x7fst/db" },
			want: "",
		},
		{
			name: "discrete name when url unset",
			set:  func(c *Config) { c.Database.Name = "discrete_db" },
			want: "discrete_db",
		},
		{
			name: "url wins over discrete name",
			set: func(c *Config) {
				c.Database.URL = "postgres://h/urldb"
				c.Database.Name = "ignored"
			},
			want: "urldb",
		},
		{
			name: "everything empty",
			set:  func(c *Config) {},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			tc.set(&c)
			if got := c.DBName(); got != tc.want {
				t.Errorf("DBName()=%q want %q", got, tc.want)
			}
		})
	}
}

// TestURLConflicts covers the boolean matrix: URL alone (no conflict), URL plus
// each discrete field (conflict), and no URL at all (no conflict regardless).
func TestURLConflicts(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Config)
		want bool
	}{
		{"url only", func(c *Config) { c.Database.URL = "postgres://h/db" }, false},
		{"url plus host", func(c *Config) { c.Database.URL, c.Database.Host = "postgres://h/db", "h" }, true},
		{"url plus user", func(c *Config) { c.Database.URL, c.Database.User = "postgres://h/db", "u" }, true},
		{"url plus name", func(c *Config) { c.Database.URL, c.Database.Name = "postgres://h/db", "n" }, true},
		{"discrete only, no url", func(c *Config) { c.Database.Host, c.Database.User = "h", "u" }, false},
		{"empty", func(c *Config) {}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c Config
			tc.set(&c)
			if got := c.URLConflicts(); got != tc.want {
				t.Errorf("URLConflicts()=%v want %v", got, tc.want)
			}
		})
	}
}

// TestDSNErrorArms hits each of the three "incomplete" error arms plus the
// password-omitted path and the SSLMode-omitted path (directly-constructed
// config with empty SSLMode omits the query parameter entirely).
func TestDSNErrorArms(t *testing.T) {
	t.Run("missing host", func(t *testing.T) {
		var c Config
		c.Database.User, c.Database.Name = "u", "db"
		_, err := c.DSN()
		if err == nil || !strings.Contains(err.Error(), "database.host is empty") {
			t.Fatalf("want host error, got %v", err)
		}
	})
	t.Run("missing user", func(t *testing.T) {
		var c Config
		c.Database.Host, c.Database.Name = "h", "db"
		_, err := c.DSN()
		if err == nil || !strings.Contains(err.Error(), "database.user is empty") {
			t.Fatalf("want user error, got %v", err)
		}
	})
	t.Run("missing name", func(t *testing.T) {
		var c Config
		c.Database.Host, c.Database.User = "h", "u"
		_, err := c.DSN()
		if err == nil || !strings.Contains(err.Error(), "database.name is empty") {
			t.Fatalf("want name error, got %v", err)
		}
	})
	t.Run("no password no port omits sslmode", func(t *testing.T) {
		var c Config
		c.Database.Host, c.Database.User, c.Database.Name = "h", "u", "db"
		// SSLMode deliberately empty; port deliberately empty.
		got, err := c.DSN()
		if err != nil {
			t.Fatal(err)
		}
		want := "postgres://u@h/db"
		if got != want {
			t.Fatalf("DSN=%q want %q", got, want)
		}
	})
}

// TestLoadErrorArms covers the two non-happy Load paths: a missing file (which
// must point at `pigration init`) and invalid YAML (which must surface a parse
// error). The generic non-IsNotExist read error is exercised by pointing Load
// at a directory, which fails to read but is not "not found".
func TestLoadErrorArms(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
		if err == nil || !strings.Contains(err.Error(), "run `pigration init`") {
			t.Fatalf("want init hint, got %v", err)
		}
	})

	t.Run("read error not IsNotExist", func(t *testing.T) {
		// A directory path is readable-as-stat but ReadFile returns a non-
		// IsNotExist error ("is a directory"), hitting the wrapped-error arm.
		dir := t.TempDir()
		_, err := Load(dir)
		if err == nil {
			t.Fatal("want error reading a directory as a file")
		}
		if strings.Contains(err.Error(), "run `pigration init`") {
			t.Fatalf("directory read should not be treated as not-found: %v", err)
		}
		if !strings.Contains(err.Error(), "reading config") {
			t.Fatalf("want reading-config wrap, got %v", err)
		}
	})

	t.Run("bad yaml", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), ".db-migration.yaml")
		if err := os.WriteFile(p, []byte("database: [this: is: not: valid"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(p)
		if err == nil || !strings.Contains(err.Error(), "parsing config") {
			t.Fatalf("want parse error, got %v", err)
		}
	})
}

// TestLoadInterpolatesDiscreteFields covers the discrete-field interpolation and
// default application arms of Load that the URL-only fixture doesn't reach
// (SSLMode default, and every discrete database.* field flowing through interp).
func TestLoadInterpolatesDiscreteFields(t *testing.T) {
	t.Setenv("COV_HOST", "db.example")
	t.Setenv("COV_USER", "alice")
	t.Setenv("COV_PASS", "secret")
	yaml := "database:\n" +
		"  host: ${COV_HOST}\n" +
		"  port: \"5432\"\n" +
		"  user: ${COV_USER}\n" +
		"  password: ${COV_PASS}\n" +
		"  name: appdb\n"
	p := filepath.Join(t.TempDir(), ".db-migration.yaml")
	if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Database.Host != "db.example" || c.Database.User != "alice" || c.Database.Password != "secret" {
		t.Fatalf("discrete fields not interpolated: %+v", c.Database)
	}
	// SSLMode default applied by Load (empty in YAML → "disable").
	if c.Database.SSLMode != "disable" {
		t.Fatalf("SSLMode default not applied: %q", c.Database.SSLMode)
	}
	got, err := c.DSN()
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://alice:secret@db.example:5432/appdb?sslmode=disable"
	if got != want {
		t.Fatalf("DSN=%q want %q", got, want)
	}
}
