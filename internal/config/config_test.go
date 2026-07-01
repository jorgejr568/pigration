package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterpolate(t *testing.T) {
	env := map[string]string{"DB_HOST": "db.local", "EMPTY": ""}
	get := func(k string) string { return env[k] }
	cases := []struct{ in, want string }{
		{"${DB_HOST}", "db.local"},
		{"${MISSING}", ""},
		{"${MISSING:-localhost}", "localhost"},
		{"${EMPTY:-fallback}", "fallback"}, // empty triggers default
		{"${DB_HOST:-x}", "db.local"},
		{"literal", "literal"},
		{"postgres://${DB_HOST}/app", "postgres://db.local/app"},
	}
	for _, c := range cases {
		if got := Interpolate(c.in, get); got != c.want {
			t.Errorf("Interpolate(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestDSN(t *testing.T) {
	var c Config
	c.Database.URL = "postgres://u:p@h/db"
	if got, _ := c.DSN(); got != "postgres://u:p@h/db" {
		t.Fatalf("url should win, got %q", got)
	}

	c = Config{}
	c.Database.Host, c.Database.Port = "h", "5432"
	c.Database.User, c.Database.Password, c.Database.Name = "u", "p", "db"
	c.Database.SSLMode = "disable"
	got, err := c.DSN()
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://u:p@h:5432/db?sslmode=disable"
	if got != want {
		t.Fatalf("DSN=%q want %q", got, want)
	}

	if _, err := (&Config{}).DSN(); err == nil {
		t.Fatal("empty config must error")
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".db-migration.yaml")
	os.WriteFile(p, []byte("database:\n  url: ${TESTURL}\n"), 0o644)
	t.Setenv("TESTURL", "postgres://x/y")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Migrations.Dir != "./migrations" || c.Migrations.Table != "schema_migrations" || c.Migrations.Package != "migrations" {
		t.Fatalf("defaults not applied: %+v", c.Migrations)
	}
	if got, _ := c.DSN(); got != "postgres://x/y" {
		t.Fatalf("interpolated DSN wrong: %q", got)
	}
}
