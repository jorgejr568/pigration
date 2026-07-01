package querybuilder

import (
	"context"
	"errors"
	"strings"
)

// CreateIndexBuilder builds a CREATE INDEX statement.
type CreateIndexBuilder struct {
	name         string
	table        string
	cols         []string
	unique       bool
	method       string
	where        string
	concurrently bool
}

// CreateIndex starts a CREATE INDEX builder.
func CreateIndex(name string) *CreateIndexBuilder {
	return &CreateIndexBuilder{name: name}
}

// On sets the target table.
func (b *CreateIndexBuilder) On(table string) *CreateIndexBuilder {
	b.table = table
	return b
}

// Columns sets the indexed columns.
func (b *CreateIndexBuilder) Columns(cols ...string) *CreateIndexBuilder {
	b.cols = append(b.cols, cols...)
	return b
}

// Unique makes the index unique.
func (b *CreateIndexBuilder) Unique() *CreateIndexBuilder {
	b.unique = true
	return b
}

// Using sets the index method (e.g. btree, gin).
func (b *CreateIndexBuilder) Using(method string) *CreateIndexBuilder {
	b.method = method
	return b
}

// Where sets a partial-index predicate (emitted verbatim).
func (b *CreateIndexBuilder) Where(pred string) *CreateIndexBuilder {
	b.where = pred
	return b
}

// Concurrently adds CONCURRENTLY.
func (b *CreateIndexBuilder) Concurrently() *CreateIndexBuilder {
	b.concurrently = true
	return b
}

// ToSQL renders the CREATE INDEX statement.
func (b *CreateIndexBuilder) ToSQL() (string, error) {
	if b.table == "" {
		return "", errors.New("querybuilder: CreateIndex requires a table (use On)")
	}
	if len(b.cols) == 0 {
		return "", errors.New("querybuilder: CreateIndex requires at least one column")
	}

	var sb strings.Builder
	sb.WriteString("CREATE ")
	if b.unique {
		sb.WriteString("UNIQUE ")
	}
	sb.WriteString("INDEX ")
	if b.concurrently {
		sb.WriteString("CONCURRENTLY ")
	}
	sb.WriteString(quoteIdent(b.name))
	sb.WriteString(" ON ")
	sb.WriteString(quoteIdent(b.table))
	if b.method != "" {
		sb.WriteString(" USING ")
		sb.WriteString(b.method)
	}
	sb.WriteString(" (")
	sb.WriteString(quoteColumnList(b.cols))
	sb.WriteString(")")
	if b.where != "" {
		sb.WriteString(" WHERE ")
		sb.WriteString(b.where)
	}
	return sb.String(), nil
}

// Execute runs the CREATE INDEX statement.
func (b *CreateIndexBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}

// DropIndexBuilder builds a DROP INDEX statement.
type DropIndexBuilder struct {
	name         string
	ifExists     bool
	concurrently bool
}

// DropIndex starts a DROP INDEX builder.
func DropIndex(name string) *DropIndexBuilder {
	return &DropIndexBuilder{name: name}
}

// IfExists adds IF EXISTS.
func (b *DropIndexBuilder) IfExists() *DropIndexBuilder {
	b.ifExists = true
	return b
}

// Concurrently adds CONCURRENTLY.
func (b *DropIndexBuilder) Concurrently() *DropIndexBuilder {
	b.concurrently = true
	return b
}

// ToSQL renders the DROP INDEX statement.
func (b *DropIndexBuilder) ToSQL() (string, error) {
	var sb strings.Builder
	sb.WriteString("DROP INDEX ")
	if b.concurrently {
		sb.WriteString("CONCURRENTLY ")
	}
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	return sb.String(), nil
}

// Execute runs the DROP INDEX statement.
func (b *DropIndexBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}
