package querybuilder

import "fmt"

// ColumnType represents a Postgres column data type.
type ColumnType struct{ sql string }

// String returns the SQL representation of the type.
func (t ColumnType) String() string { return t.sql }

// Value-typed column types.
var (
	SmallInt    = ColumnType{"smallint"}
	Int         = ColumnType{"integer"}
	BigInt      = ColumnType{"bigint"}
	Serial      = ColumnType{"serial"}
	BigSerial   = ColumnType{"bigserial"}
	Real        = ColumnType{"real"}
	Double      = ColumnType{"double precision"}
	Text        = ColumnType{"text"}
	Bool        = ColumnType{"boolean"}
	Date        = ColumnType{"date"}
	Time        = ColumnType{"time"}
	Timestamp   = ColumnType{"timestamp"}
	Timestamptz = ColumnType{"timestamptz"}
	UUID        = ColumnType{"uuid"}
	JSON        = ColumnType{"json"}
	JSONB       = ColumnType{"jsonb"}
	Bytea       = ColumnType{"bytea"}
	Inet        = ColumnType{"inet"}
)

// Varchar returns a varchar(n) type.
func Varchar(n int) ColumnType {
	return ColumnType{fmt.Sprintf("varchar(%d)", n)}
}

// Char returns a char(n) type.
func Char(n int) ColumnType {
	return ColumnType{fmt.Sprintf("char(%d)", n)}
}

// Numeric returns a numeric(precision,scale) type.
func Numeric(precision, scale int) ColumnType {
	return ColumnType{fmt.Sprintf("numeric(%d,%d)", precision, scale)}
}
