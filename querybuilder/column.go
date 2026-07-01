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
	onDelete *Action
	onUpdate *Action
}

// FKOption configures a foreign-key reference.
type FKOption func(*fkRef)

// WithOnDelete sets the ON DELETE referential action.
func WithOnDelete(a Action) FKOption {
	return func(r *fkRef) { r.onDelete = &a }
}

// WithOnUpdate sets the ON UPDATE referential action.
func WithOnUpdate(a Action) FKOption {
	return func(r *fkRef) { r.onUpdate = &a }
}

// columnDef holds the accumulated definition of a single column.
type columnDef struct {
	name          string
	typ           ColumnType
	notNull       bool
	unique        bool
	primaryKey    bool
	autoIncrement bool
	unsigned      bool
	def           *string
	check         *string
	comment       *string
	generated     *string
	fk            *fkRef
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

// WithAutoIncrement maps integer types to serial/bigserial.
func WithAutoIncrement() ColumnModifier {
	return func(c *columnDef) { c.autoIncrement = true }
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
		var s string
		switch x := v.(type) {
		case string:
			s = quoteLiteral(x)
		case Expr:
			s = x.expr()
		default:
			s = fmt.Sprint(x)
		}
		c.def = &s
	}
}

// Check adds a CHECK constraint expression (emitted verbatim).
func Check(expr string) ColumnModifier {
	return func(c *columnDef) { c.check = &expr }
}

// Comment attaches a comment to the column.
func Comment(text string) ColumnModifier {
	return func(c *columnDef) { c.comment = &text }
}

// GeneratedAs makes the column a generated stored column.
func GeneratedAs(expr string) ColumnModifier {
	return func(c *columnDef) { c.generated = &expr }
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

// typeSQL returns the type SQL, mapping to serial/bigserial when
// autoIncrement is set.
func (c columnDef) typeSQL() string {
	if c.autoIncrement {
		switch c.typ.String() {
		case "integer", "smallint":
			return "serial"
		case "bigint":
			return "bigserial"
		}
	}
	return c.typ.String()
}

// definitionSQL renders the column definition, e.g.
// `"age" integer NOT NULL CHECK ("age" >= 0)`.
func (c columnDef) definitionSQL() (string, error) {
	parts := []string{quoteIdent(c.name), c.typeSQL()}

	if c.primaryKey {
		parts = append(parts, "PRIMARY KEY")
	}
	if c.notNull {
		parts = append(parts, "NOT NULL")
	}
	if c.unique {
		parts = append(parts, "UNIQUE")
	}
	if c.def != nil {
		parts = append(parts, "DEFAULT "+*c.def)
	}
	if c.fk != nil {
		fk := "REFERENCES " + quoteIdent(c.fk.table) + " (" + quoteIdent(c.fk.column) + ")"
		if c.fk.onDelete != nil {
			fk += " ON DELETE " + string(*c.fk.onDelete)
		}
		if c.fk.onUpdate != nil {
			fk += " ON UPDATE " + string(*c.fk.onUpdate)
		}
		parts = append(parts, fk)
	}
	if c.generated != nil {
		parts = append(parts, "GENERATED ALWAYS AS ("+*c.generated+") STORED")
	}
	// Combine explicit CHECK and unsigned CHECK.
	var checks []string
	if c.check != nil {
		checks = append(checks, "CHECK ("+*c.check+")")
	}
	if c.unsigned {
		checks = append(checks, "CHECK ("+quoteIdent(c.name)+" >= 0)")
	}
	parts = append(parts, checks...)

	return strings.Join(parts, " "), nil
}
