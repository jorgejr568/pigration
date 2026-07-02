package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
	"github.com/jorgejr568/pigration/querybuilder"
)

func CreateTodos1782900000Up(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.CreateTable("todos").
		ID("id", querybuilder.BigSerial).
		Column("title", querybuilder.Text, querybuilder.NotNull()).
		Column("done", querybuilder.Bool, querybuilder.NotNull(), querybuilder.Default(false)).
		Timestamps().
		Execute(ctx, tx)
}

func CreateTodos1782900000Down(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.DropTable("todos").IfExists().Execute(ctx, tx)
}

func init() {
	migrator.Register("1782900000_create_todos",
		CreateTodos1782900000Up, CreateTodos1782900000Down)
}
