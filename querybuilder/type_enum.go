package querybuilder

import (
	"context"
	"errors"
	"strings"
)

// CreateTypeBuilder builds a CREATE TYPE statement.
type CreateTypeBuilder struct {
	name       string
	enumValues []string
}

// CreateType starts a CREATE TYPE builder.
func CreateType(name string) *CreateTypeBuilder {
	return &CreateTypeBuilder{name: name}
}

// AsEnum declares the type as an enum with the given values.
func (b *CreateTypeBuilder) AsEnum(vals ...string) *CreateTypeBuilder {
	b.enumValues = append(b.enumValues, vals...)
	return b
}

// ToSQL renders the CREATE TYPE statement.
func (b *CreateTypeBuilder) ToSQL() (string, error) {
	if len(b.enumValues) == 0 {
		return "", errors.New("querybuilder: CreateType requires at least one enum value (use AsEnum)")
	}
	quoted := make([]string, len(b.enumValues))
	for i, v := range b.enumValues {
		quoted[i] = quoteLiteral(v)
	}
	return "CREATE TYPE " + quoteIdent(b.name) + " AS ENUM (" + strings.Join(quoted, ", ") + ")", nil
}

// Execute runs the CREATE TYPE statement.
func (b *CreateTypeBuilder) Execute(ctx context.Context, exec Execer) error {
	return execBuilder(ctx, exec, b)
}

// AlterTypeBuilder builds an ALTER TYPE ... ADD VALUE statement.
type AlterTypeBuilder struct {
	typeName string
	value    string
	before   string
	after    string
}

// AlterTypeAddValue starts an ALTER TYPE ... ADD VALUE builder.
func AlterTypeAddValue(typeName, value string) *AlterTypeBuilder {
	return &AlterTypeBuilder{typeName: typeName, value: value}
}

// Before places the new value before an existing value.
func (b *AlterTypeBuilder) Before(v string) *AlterTypeBuilder {
	b.before = v
	b.after = ""
	return b
}

// After places the new value after an existing value.
func (b *AlterTypeBuilder) After(v string) *AlterTypeBuilder {
	b.after = v
	b.before = ""
	return b
}

// ToSQL renders the ALTER TYPE ... ADD VALUE statement.
func (b *AlterTypeBuilder) ToSQL() (string, error) {
	sql := "ALTER TYPE " + quoteIdent(b.typeName) + " ADD VALUE " + quoteLiteral(b.value)
	if b.before != "" {
		sql += " BEFORE " + quoteLiteral(b.before)
	} else if b.after != "" {
		sql += " AFTER " + quoteLiteral(b.after)
	}
	return sql, nil
}

// Execute runs the ALTER TYPE statement.
func (b *AlterTypeBuilder) Execute(ctx context.Context, exec Execer) error {
	return execBuilder(ctx, exec, b)
}
