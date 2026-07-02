package querybuilder_test

import (
	"fmt"

	"github.com/jorgejr568/pigration/querybuilder"
)

func ExampleCreateTable() {
	sql, _ := querybuilder.CreateTable("users").
		ID("id", querybuilder.BigSerial).
		Column("email", querybuilder.Text, querybuilder.NotNull(), querybuilder.Unique()).
		Column("age", querybuilder.Int, querybuilder.WithUnsigned()).
		Timestamps().
		ToSQL()
	fmt.Println(sql)
	// Output:
	// CREATE TABLE "users" ( "id" bigserial PRIMARY KEY, "email" text NOT NULL UNIQUE, "age" integer CHECK ("age" >= 0), "created_at" timestamptz NOT NULL DEFAULT now(), "updated_at" timestamptz NOT NULL DEFAULT now() )
}

func ExampleCreateTable_foreignKey() {
	sql, _ := querybuilder.CreateTable("posts").
		ID("id", querybuilder.BigSerial).
		Column("author_id", querybuilder.BigInt,
			querybuilder.NotNull(),
			querybuilder.References("users", "id", querybuilder.WithOnDelete(querybuilder.Cascade))).
		ToSQL()
	fmt.Println(sql)
	// Output:
	// CREATE TABLE "posts" ( "id" bigserial PRIMARY KEY, "author_id" bigint NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE )
}

func ExampleAlterTable() {
	sql, _ := querybuilder.AlterTable("users").
		AddColumn("phone", querybuilder.Varchar(32), querybuilder.NotNull()).
		DropColumn("legacy").
		ToSQL()
	fmt.Println(sql)
	// Output:
	// ALTER TABLE "users" ADD COLUMN "phone" varchar(32) NOT NULL, DROP COLUMN "legacy"
}

func ExampleCreateIndex() {
	sql, _ := querybuilder.CreateIndex("idx_users_email").
		On("users").
		Columns("email").
		Unique().
		Where("deleted_at IS NULL").
		ToSQL()
	fmt.Println(sql)
	// Output:
	// CREATE UNIQUE INDEX "idx_users_email" ON "users" ("email") WHERE deleted_at IS NULL
}

func ExampleCreateType() {
	sql, _ := querybuilder.CreateType("user_role").
		AsEnum("admin", "member", "guest").
		ToSQL()
	fmt.Println(sql)
	// Output:
	// CREATE TYPE "user_role" AS ENUM ('admin', 'member', 'guest')
}
