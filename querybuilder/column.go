package querybuilder

import (
	"fmt"
	"strings"
)

// Action is a foreign-key referential action.
type Action string

const (
	Cascade    Action = "CASCADE"
	SetNull    Action = "SET NULL"
	Restrict   Action = "RESTRICT"
	NoAction   Action = "NO ACTION"
	SetDefault Action = "SET DEFAULT"
)

// fkRef holds a column's foreign-key reference.
type fkRef struct {
	table    string
	column   string
	onDelete Action
	onUpdate Action
}

// referencesSQL renders the REFERENCES fragment, e.g.
// `REFERENCES "orgs" ("id") ON DELETE CASCADE`.
func (r fkRef) referencesSQL() string {
	sql := "REFERENCES " + quoteIdent(r.table) + " (" + quoteIdent(r.column) + ")"
	if r.onDelete != "" {
		sql += " ON DELETE " + string(r.onDelete)
	}
	if r.onUpdate != "" {
		sql += " ON UPDATE " + string(r.onUpdate)
	}
	return sql
}

// FKOption configures a foreign-key reference.
type FKOption func(*fkRef)

// WithOnDelete sets the ON DELETE referential action.
func WithOnDelete(a Action) FKOption {
	return func(r *fkRef) { r.onDelete = a }
}

// WithOnUpdate sets the ON UPDATE referential action.
func WithOnUpdate(a Action) FKOption {
	return func(r *fkRef) { r.onUpdate = a }
}

// columnDef holds the accumulated definition of a single column.
type columnDef struct {
	name       string
	typ        ColumnType
	notNull    bool
	unique     bool
	primaryKey bool
	unsigned   bool
	def        string
	check      string
	generated  string
	fk         *fkRef
}

// ColumnModifier mutates a columnDef.
type ColumnModifier func(*columnDef)

// NotNull marks the column NOT NULL.
func NotNull() ColumnModifier {
	return func(c *columnDef) { c.notNull = true }
}

// Unique marks the column UNIQUE.
func Unique() ColumnModifier {
	return func(c *columnDef) { c.unique = true }
}

// PrimaryKey marks the column as the primary key.
func PrimaryKey() ColumnModifier {
	return func(c *columnDef) { c.primaryKey = true }
}

// WithUnsigned emits a CHECK (col >= 0) constraint (Postgres has no unsigned ints).
func WithUnsigned() ColumnModifier {
	return func(c *columnDef) { c.unsigned = true }
}

// Default sets a column default. Strings are quoted as literals, Expr values
// are emitted verbatim, and other values (numbers, bools) use their Go
// representation.
func Default(v any) ColumnModifier {
	return func(c *columnDef) {
		switch x := v.(type) {
		case string:
			c.def = quoteLiteral(x)
		case Expr:
			c.def = x.expr()
		default:
			c.def = fmt.Sprint(x)
		}
	}
}

// Check adds a CHECK constraint expression (emitted verbatim).
func Check(expr string) ColumnModifier {
	return func(c *columnDef) { c.check = expr }
}

// GeneratedAs makes the column a generated stored column.
func GeneratedAs(expr string) ColumnModifier {
	return func(c *columnDef) { c.generated = expr }
}

// References adds a foreign-key reference to the column.
func References(table, column string, opts ...FKOption) ColumnModifier {
	return func(c *columnDef) {
		ref := &fkRef{table: table, column: column}
		for _, o := range opts {
			o(ref)
		}
		c.fk = ref
	}
}

// definitionSQL renders the column definition, e.g.
// `"age" integer NOT NULL CHECK ("age" >= 0)`.
func (c columnDef) definitionSQL() string {
	parts := []string{quoteIdent(c.name), c.typ.String()}

	if c.primaryKey {
		parts = append(parts, "PRIMARY KEY")
	}
	if c.notNull {
		parts = append(parts, "NOT NULL")
	}
	if c.unique {
		parts = append(parts, "UNIQUE")
	}
	if c.def != "" {
		parts = append(parts, "DEFAULT "+c.def)
	}
	if c.fk != nil {
		parts = append(parts, c.fk.referencesSQL())
	}
	if c.generated != "" {
		parts = append(parts, "GENERATED ALWAYS AS ("+c.generated+") STORED")
	}
	// Combine explicit CHECK and unsigned CHECK.
	var checks []string
	if c.check != "" {
		checks = append(checks, "CHECK ("+c.check+")")
	}
	if c.unsigned {
		checks = append(checks, "CHECK ("+quoteIdent(c.name)+" >= 0)")
	}
	parts = append(parts, checks...)

	return strings.Join(parts, " ")
}
