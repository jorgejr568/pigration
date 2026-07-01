package querybuilder

import (
	"context"
	"errors"
	"strings"
)

// alterAction is a single ALTER TABLE action. If standalone is true it must be
// emitted as its own ALTER TABLE statement (e.g. RENAME).
type alterAction struct {
	clause     string
	err        error
	standalone bool
}

// AlterTableBuilder builds one or more ALTER TABLE statements.
type AlterTableBuilder struct {
	name    string
	actions []alterAction
}

// AlterTable starts an ALTER TABLE builder.
func AlterTable(name string) *AlterTableBuilder {
	return &AlterTableBuilder{name: name}
}

func (b *AlterTableBuilder) add(clause string) *AlterTableBuilder {
	b.actions = append(b.actions, alterAction{clause: clause})
	return b
}

func (b *AlterTableBuilder) addStandalone(clause string) *AlterTableBuilder {
	b.actions = append(b.actions, alterAction{clause: clause, standalone: true})
	return b
}

// AddColumn adds a column.
func (b *AlterTableBuilder) AddColumn(name string, t ColumnType, mods ...ColumnModifier) *AlterTableBuilder {
	c := columnDef{name: name, typ: t}
	for _, m := range mods {
		m(&c)
	}
	def, err := c.definitionSQL()
	if err != nil {
		b.actions = append(b.actions, alterAction{err: err})
		return b
	}
	return b.add("ADD COLUMN " + def)
}

// DropColumn drops a column.
func (b *AlterTableBuilder) DropColumn(name string) *AlterTableBuilder {
	return b.add("DROP COLUMN " + quoteIdent(name))
}

// RenameColumn renames a column (emitted as a separate statement).
func (b *AlterTableBuilder) RenameColumn(from, to string) *AlterTableBuilder {
	return b.addStandalone("RENAME COLUMN " + quoteIdent(from) + " TO " + quoteIdent(to))
}

// AlterColumnType changes a column's type.
func (b *AlterTableBuilder) AlterColumnType(name string, t ColumnType) *AlterTableBuilder {
	return b.add("ALTER COLUMN " + quoteIdent(name) + " TYPE " + t.String())
}

// SetDefault sets a column default (expr emitted verbatim).
func (b *AlterTableBuilder) SetDefault(name string, expr string) *AlterTableBuilder {
	return b.add("ALTER COLUMN " + quoteIdent(name) + " SET DEFAULT " + expr)
}

// DropDefault drops a column default.
func (b *AlterTableBuilder) DropDefault(name string) *AlterTableBuilder {
	return b.add("ALTER COLUMN " + quoteIdent(name) + " DROP DEFAULT")
}

// SetNotNull sets NOT NULL on a column.
func (b *AlterTableBuilder) SetNotNull(name string) *AlterTableBuilder {
	return b.add("ALTER COLUMN " + quoteIdent(name) + " SET NOT NULL")
}

// DropNotNull drops NOT NULL from a column.
func (b *AlterTableBuilder) DropNotNull(name string) *AlterTableBuilder {
	return b.add("ALTER COLUMN " + quoteIdent(name) + " DROP NOT NULL")
}

// AddConstraint adds a named constraint (expr is the full constraint body).
func (b *AlterTableBuilder) AddConstraint(name, expr string) *AlterTableBuilder {
	return b.add("ADD CONSTRAINT " + quoteIdent(name) + " " + expr)
}

// DropConstraint drops a named constraint.
func (b *AlterTableBuilder) DropConstraint(name string) *AlterTableBuilder {
	return b.add("DROP CONSTRAINT " + quoteIdent(name))
}

// AddForeignKey adds a foreign-key constraint named fk_<table>_<column>.
func (b *AlterTableBuilder) AddForeignKey(column, refTable, refColumn string, opts ...FKOption) *AlterTableBuilder {
	ref := &fkRef{table: refTable, column: refColumn}
	for _, o := range opts {
		o(ref)
	}
	clause := "ADD CONSTRAINT " + quoteIdent("fk_"+b.name+"_"+column) +
		" FOREIGN KEY (" + quoteIdent(column) + ")" +
		" REFERENCES " + quoteIdent(refTable) + " (" + quoteIdent(refColumn) + ")"
	if ref.onDelete != nil {
		clause += " ON DELETE " + string(*ref.onDelete)
	}
	if ref.onUpdate != nil {
		clause += " ON UPDATE " + string(*ref.onUpdate)
	}
	return b.add(clause)
}

// RenameTo renames the table (emitted as a separate statement).
func (b *AlterTableBuilder) RenameTo(newName string) *AlterTableBuilder {
	return b.addStandalone("RENAME TO " + quoteIdent(newName))
}

// buildStatements assembles the ALTER TABLE statements as separate strings.
// Combinable actions are merged into one comma-separated statement; standalone
// actions (e.g. RENAME) each get their own statement.
func (b *AlterTableBuilder) buildStatements() ([]string, error) {
	if len(b.actions) == 0 {
		return nil, errors.New("querybuilder: AlterTable requires at least one action")
	}

	prefix := "ALTER TABLE " + quoteIdent(b.name)
	var stmts []string
	var combinable []string

	flush := func() {
		if len(combinable) > 0 {
			stmts = append(stmts, prefix+" "+strings.Join(combinable, ", "))
			combinable = nil
		}
	}

	for _, a := range b.actions {
		if a.err != nil {
			return nil, a.err
		}
		if a.standalone {
			flush()
			stmts = append(stmts, prefix+" "+a.clause)
			continue
		}
		combinable = append(combinable, a.clause)
	}
	flush()

	return stmts, nil
}

// ToSQL renders the ALTER TABLE statement(s), joined by "; " for display.
func (b *AlterTableBuilder) ToSQL() (string, error) {
	stmts, err := b.buildStatements()
	if err != nil {
		return "", err
	}
	return strings.Join(stmts, "; "), nil
}

// statements returns each statement separately for execution. It builds the
// slice directly rather than splitting ToSQL's joined output, so a clause body
// that itself contains "; " (e.g. a default literal) is never torn apart.
func (b *AlterTableBuilder) statements() ([]string, error) {
	return b.buildStatements()
}

// Execute runs the ALTER TABLE statement(s).
func (b *AlterTableBuilder) Execute(ctx context.Context, exec Execer) error {
	stmts, err := b.statements()
	if err != nil {
		return err
	}
	return execStatements(ctx, exec, stmts)
}
