package querybuilder

import (
	"context"
	"strings"
)

// DropBuilder builds a DROP TABLE/TYPE/SCHEMA statement.
type DropBuilder struct {
	kind     string // "TABLE", "TYPE", or "SCHEMA"
	name     string
	ifExists bool
	cascade  bool
}

// DropTable starts a DROP TABLE builder.
func DropTable(name string) *DropBuilder { return &DropBuilder{kind: "TABLE", name: name} }

// DropType starts a DROP TYPE builder.
func DropType(name string) *DropBuilder { return &DropBuilder{kind: "TYPE", name: name} }

// DropSchema starts a DROP SCHEMA builder.
func DropSchema(name string) *DropBuilder { return &DropBuilder{kind: "SCHEMA", name: name} }

// IfExists adds IF EXISTS.
func (b *DropBuilder) IfExists() *DropBuilder {
	b.ifExists = true
	return b
}

// Cascade adds CASCADE.
func (b *DropBuilder) Cascade() *DropBuilder {
	b.cascade = true
	return b
}

// ToSQL renders the DROP statement.
func (b *DropBuilder) ToSQL() (string, error) {
	var sb strings.Builder
	sb.WriteString("DROP ")
	sb.WriteString(b.kind)
	sb.WriteString(" ")
	if b.ifExists {
		sb.WriteString("IF EXISTS ")
	}
	sb.WriteString(quoteIdent(b.name))
	if b.cascade {
		sb.WriteString(" CASCADE")
	}
	return sb.String(), nil
}

// Execute runs the DROP statement.
func (b *DropBuilder) Execute(ctx context.Context, exec Execer) error {
	return execBuilder(ctx, exec, b)
}
