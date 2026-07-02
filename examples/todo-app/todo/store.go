// Package todo is the storage and HTTP layer of the pigration example app.
package todo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Todo struct {
	ID        int64      `json:"id"`
	Title     string     `json:"title"`
	Done      bool       `json:"done"`
	DueDate   *time.Time `json:"due_date"`
	CreatedAt time.Time  `json:"created_at"`
}

var ErrNotFound = errors.New("todo not found")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const todoColumns = `id, title, done, due_date, created_at`

func (s *Store) List(ctx context.Context) ([]Todo, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+todoColumns+` FROM todos ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var todos []Todo
	for rows.Next() {
		var t Todo
		if err := rows.Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt); err != nil {
			return nil, err
		}
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (s *Store) Create(ctx context.Context, title string) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`INSERT INTO todos (title) VALUES ($1) RETURNING `+todoColumns, title).
		Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt)
	return t, err
}

func (s *Store) Toggle(ctx context.Context, id int64) (Todo, error) {
	var t Todo
	err := s.pool.QueryRow(ctx,
		`UPDATE todos SET done = NOT done WHERE id = $1 RETURNING `+todoColumns, id).
		Scan(&t.ID, &t.Title, &t.Done, &t.DueDate, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Todo{}, ErrNotFound
	}
	return t, err
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM todos WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
