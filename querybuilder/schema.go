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
	return execBuilder(ctx, exec, b)
}
