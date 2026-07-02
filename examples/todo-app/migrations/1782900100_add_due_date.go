package migrations

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
)

func AddDueDate1782900100Up(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, `ALTER TABLE todos ADD COLUMN due_date date;`)
	return err
}

func AddDueDate1782900100Down(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, `ALTER TABLE todos DROP COLUMN due_date;`)
	return err
}

func init() {
	migrator.Register("1782900100_add_due_date",
		AddDueDate1782900100Up, AddDueDate1782900100Down)
}
