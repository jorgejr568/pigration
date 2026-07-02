package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
	"github.com/jorgejr568/pigration/querybuilder"
)

func AddPendingIndex1782900200Up(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.CreateIndex("idx_todos_pending").
		On("todos").Columns("done").
		Where("done = false").
		Concurrently().
		Execute(ctx, tx)
}

func AddPendingIndex1782900200Down(ctx context.Context, tx migrator.Executor) error {
	return querybuilder.DropIndex("idx_todos_pending").IfExists().Concurrently().Execute(ctx, tx)
}

func init() {
	// CREATE INDEX CONCURRENTLY cannot run inside a transaction.
	migrator.Register("1782900200_add_pending_index",
		AddPendingIndex1782900200Up, AddPendingIndex1782900200Down,
		migrator.NonTransactional())
}
