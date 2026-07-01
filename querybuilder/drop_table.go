package querybuilder

import (
	"context"
	"strings"
)

// DropTableBuilder builds a DROP TABLE statement.
type DropTableBuilder struct {
	name     string
	ifExists bool
	cascade  bool
}

// DropTable starts a DROP TABLE builder.
func DropTable(name string) *DropTableBuilder {
	return &DropTableBuilder{name: name}
}

// IfExists adds IF EXISTS.
func (b *DropTableBuilder) IfExists() *DropTableBuilder {
	b.ifExists = true
	return b
}

// Cascade adds CASCADE.
func (b *DropTableBuilder) Cascade() *DropTableBuilder {
	b.cascade = true
	return b
}

// ToSQL renders the DROP TABLE statement.
func (b *DropTableBuilder) ToSQL() (string, error) {
	var sb strings.Builder
	sb.WriteString("DROP TABLE ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	if b.cascade {
		sb.WriteString(" CASCADE")
	}
	return sb.String(), nil
}

// Execute runs the DROP TABLE statement.
func (b *DropTableBuilder) Execute(ctx context.Context, exec Execer) error {
	sql, err := b.ToSQL()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, []string{sql})
}
