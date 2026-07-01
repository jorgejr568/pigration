package codegen

import (
	"strings"
	"testing"
)

func TestRenderMigrationNaming(t *testing.T) {
	fname, content, err := RenderMigration("migrations", "Create Users", 1719800000)
	if err != nil {
		t.Fatal(err)
	}
	if fname != "1719800000_create_users.go" {
		t.Fatalf("filename=%q", fname)
	}
	for _, want := range []string{
		"package migrations",
		"func CreateUsers1719800000Up(ctx context.Context, tx migrator.Executor) error",
		"func CreateUsers1719800000Down(ctx context.Context, tx migrator.Executor) error",
		`migrator.Register("1719800000_create_users",`,
		`"github.com/jorgejr568/pigration/migrator"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestSnakeCase(t *testing.T) {
	cases := map[string]string{
		"Create Users":  "create_users",
		"CreateUsers":   "create_users",
		"create-users":  "create_users",
		"AddIndexToFoo": "add_index_to_foo",
		"  spaced  out": "spaced_out",
	}
	for in, want := range cases {
		if got := SnakeCase(in); got != want {
			t.Errorf("SnakeCase(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCamelCase(t *testing.T) {
	cases := map[string]string{
		"create users": "CreateUsers",
		"create_users": "CreateUsers",
		"create-users": "CreateUsers",
	}
	for in, want := range cases {
		if got := CamelCase(in); got != want {
			t.Errorf("CamelCase(%q)=%q want %q", in, got, want)
		}
	}
}
