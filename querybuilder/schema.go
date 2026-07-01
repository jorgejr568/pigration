package querybuilder

import (
	"context"
	"strings"
)

// CreateSchemaBuilder builds a CREATE SCHEMA statement.
type CreateSchemaBuilder struct {
	name        string
	ifNotExists bool
}

// CreateSchema starts a CREATE SCHEMA builder.
func CreateSchema(name string) *CreateSchemaBuilder {
	return &CreateSchemaBuilder{name: name}
}

// IfNotExists adds IF NOT EXISTS.
func (b *CreateSchemaBuilder) IfNotExists() *CreateSchemaBuilder {
	b.ifNotExists = true
	return b
}

// ToSQL renders the CREATE SCHEMA statement.
func (b *CreateSchemaBuilder) ToSQL() (string, error) {
	var sb strings.Builder
	sb.WriteString("CREATE SCHEMA ")
	if b.ifNotExists {
		sb.WriteString("IF NOT EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	return sb.String(), nil
}

// Execute runs the CREATE SCHEMA statement.
func (b *CreateSchemaBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}

// DropSchemaBuilder builds a DROP SCHEMA statement.
type DropSchemaBuilder struct {
	name     string
	ifExists bool
	cascade  bool
}

// DropSchema starts a DROP SCHEMA builder.
func DropSchema(name string) *DropSchemaBuilder {
	return &DropSchemaBuilder{name: name}
}

// IfExists adds IF EXISTS.
func (b *DropSchemaBuilder) IfExists() *DropSchemaBuilder {
	b.ifExists = true
	return b
}

// Cascade adds CASCADE.
func (b *DropSchemaBuilder) Cascade() *DropSchemaBuilder {
	b.cascade = true
	return b
}

// ToSQL renders the DROP SCHEMA statement.
func (b *DropSchemaBuilder) ToSQL() (string, error) {
	var sb strings.Builder
	sb.WriteString("DROP SCHEMA ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	if b.cascade {
		sb.WriteString(" CASCADE")
	}
	return sb.String(), nil
}

// Execute runs the DROP SCHEMA statement.
func (b *DropSchemaBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}
