package querybuilder

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationRoundtrip(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx := context.Background()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Clean up any leftovers from a prior failed run.
	_ = DropTable("qb_users").IfExists().Cascade().Execute(ctx, pool)
	_ = DropType("qb_role").IfExists().Cascade().Execute(ctx, pool)
	_ = DropSchema("qb_billing").IfExists().Cascade().Execute(ctx, pool)

	must(CreateSchema("qb_billing").IfNotExists().Execute(ctx, pool))
	must(CreateType("qb_role").AsEnum("admin", "member", "guest").Execute(ctx, pool))
	must(CreateTable("qb_users").
		ID("id", BigSerial).
		Column("email", Text, NotNull(), Unique()).
		Column("age", Int, WithUnsigned()).
		Timestamps().Execute(ctx, pool))
	must(CreateIndex("qb_idx_email").On("qb_users").Columns("email").Unique().Execute(ctx, pool))
	must(AlterTable("qb_users").AddColumn("phone", Varchar(32)).Execute(ctx, pool))

	// Tear down.
	must(DropTable("qb_users").IfExists().Cascade().Execute(ctx, pool))
	must(DropType("qb_role").IfExists().Cascade().Execute(ctx, pool))
	must(DropSchema("qb_billing").IfExists().Cascade().Execute(ctx, pool))
}
