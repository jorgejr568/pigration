package querybuilder

import (
	"strings"
	"testing"
)

func TestAlterTableActions(t *testing.T) {
	sql, err := AlterTable("users").
		AddColumn("phone", Varchar(32), NotNull()).
		DropColumn("legacy").
		AlterColumnType("age", BigInt).
		SetNotNull("phone").
		AddConstraint("chk_age", "CHECK (age >= 0)").
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	n := norm(sql)
	for _, want := range []string{
		`ALTER TABLE "users"`,
		`ADD COLUMN "phone" varchar(32) NOT NULL`,
		`DROP COLUMN "legacy"`,
		`ALTER COLUMN "age" TYPE bigint`,
		`ALTER COLUMN "phone" SET NOT NULL`,
		`ADD CONSTRAINT "chk_age" CHECK (age >= 0)`,
	} {
		if !strings.Contains(n, want) {
			t.Errorf("missing %q in %q", want, n)
		}
	}
}

func TestRenameEmitsSeparateStatement(t *testing.T) {
	sql, _ := AlterTable("users").RenameColumn("email", "email_address").ToSQL()
	if !strings.Contains(norm(sql), `RENAME COLUMN "email" TO "email_address"`) {
		t.Fatal("rename column")
	}
	sql2, _ := AlterTable("users").RenameTo("app_users").ToSQL()
	if !strings.Contains(norm(sql2), `ALTER TABLE "users" RENAME TO "app_users"`) {
		t.Fatal("rename table")
	}
}

func TestAlterTableMoreActions(t *testing.T) {
	sql, err := AlterTable("users").
		SetDefault("status", "'active'").
		DropDefault("status").
		DropNotNull("phone").
		DropConstraint("chk_age").
		AddForeignKey("org_id", "orgs", "id", WithOnDelete(Cascade)).
		ToSQL()
	if err != nil {
		t.Fatal(err)
	}
	n := norm(sql)
	for _, want := range []string{
		`ALTER COLUMN "status" SET DEFAULT 'active'`,
		`ALTER COLUMN "status" DROP DEFAULT`,
		`ALTER COLUMN "phone" DROP NOT NULL`,
		`DROP CONSTRAINT "chk_age"`,
		`ADD CONSTRAINT "fk_users_org_id" FOREIGN KEY ("org_id") REFERENCES "orgs" ("id") ON DELETE CASCADE`,
	} {
		if !strings.Contains(n, want) {
			t.Errorf("missing %q in %q", want, n)
		}
	}
}

func TestAlterTableNoActionsErrors(t *testing.T) {
	if _, err := AlterTable("users").ToSQL(); err == nil {
		t.Fatal("expected error for no actions")
	}
}
