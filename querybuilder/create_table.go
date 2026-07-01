package querybuilder

import (
	"context"
	"errors"
	"strings"
)

// CreateTableBuilder builds a CREATE TABLE statement.
type CreateTableBuilder struct {
	name             string
	ifNotExists      bool
	cols             []columnDef
	tableConstraints []string
}

// CreateTable starts a CREATE TABLE builder.
func CreateTable(name string) *CreateTableBuilder {
	return &CreateTableBuilder{name: name}
}

// IfNotExists adds IF NOT EXISTS.
func (b *CreateTableBuilder) IfNotExists() *CreateTableBuilder {
	b.ifNotExists = true
	return b
}

// Column adds a column with optional modifiers.
func (b *CreateTableBuilder) Column(name string, t ColumnType, mods ...ColumnModifier) *CreateTableBuilder {
	c := columnDef{name: name, typ: t}
	for _, m := range mods {
		m(&c)
	}
	b.cols = append(b.cols, c)
	return b
}

// ID adds a primary-key column.
func (b *CreateTableBuilder) ID(name string, t ColumnType, mods ...ColumnModifier) *CreateTableBuilder {
	return b.Column(name, t, append(mods, PrimaryKey())...)
}

// UUIDPrimary adds a uuid primary key defaulting to gen_random_uuid().
func (b *CreateTableBuilder) UUIDPrimary(name string) *CreateTableBuilder {
	return b.Column(name, UUID, PrimaryKey(), Default(RawExpr("gen_random_uuid()")))
}

// Timestamps adds created_at and updated_at timestamptz columns.
func (b *CreateTableBuilder) Timestamps() *CreateTableBuilder {
	b.Column("created_at", Timestamptz, NotNull(), Default(RawExpr("now()")))
	b.Column("updated_at", Timestamptz, NotNull(), Default(RawExpr("now()")))
	return b
}

// PrimaryKeyColumns adds a composite primary key table constraint.
func (b *CreateTableBuilder) PrimaryKeyColumns(cols ...string) *CreateTableBuilder {
	b.tableConstraints = append(b.tableConstraints, "PRIMARY KEY ("+quoteColumnList(cols)+")")
	return b
}

// UniqueColumns adds a composite unique table constraint.
func (b *CreateTableBuilder) UniqueColumns(cols ...string) *CreateTableBuilder {
	b.tableConstraints = append(b.tableConstraints, "UNIQUE ("+quoteColumnList(cols)+")")
	return b
}

// CheckConstraint adds a named CHECK table constraint.
func (b *CreateTableBuilder) CheckConstraint(name, expr string) *CreateTableBuilder {
	b.tableConstraints = append(b.tableConstraints, "CONSTRAINT "+quoteIdent(name)+" CHECK ("+expr+")")
	return b
}

// ToSQL renders the CREATE TABLE statement.
func (b *CreateTableBuilder) ToSQL() (string, error) {
	if len(b.cols) == 0 {
		return "", errors.New("querybuilder: CreateTable requires at least one column")
	}
	parts := make([]string, 0, len(b.cols)+len(b.tableConstraints))
	for _, c := range b.cols {
		def, err := c.definitionSQL()
		if err != nil {
			return "", err
		}
		parts = append(parts, def)
	}
	parts = append(parts, b.tableConstraints...)

	var sb strings.Builder
	sb.WriteString("CREATE TABLE ")
	if b.ifNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	sb.WriteString(" ( ")
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(" )")
	return sb.String(), nil
}

// Execute runs the CREATE TABLE statement.
func (b *CreateTableBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}

// quoteColumnList quotes each column identifier and joins with commas.
func quoteColumnList(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	return strings.Join(quoted, ", ")
}
