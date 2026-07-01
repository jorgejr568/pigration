package querybuilder

import "testing"

func TestColumnTypes(t *testing.T) {
	cases := map[string]ColumnType{
		"bigint":        BigInt,
		"text":          Text,
		"boolean":       Bool,
		"timestamptz":   Timestamptz,
		"uuid":          UUID,
		"jsonb":         JSONB,
		"varchar(255)":  Varchar(255),
		"numeric(10,2)": Numeric(10, 2),
	}
	for want, ct := range cases {
		if ct.String() != want {
			t.Errorf("got %q want %q", ct.String(), want)
		}
	}
}

func TestColumnTypesExtra(t *testing.T) {
	cases := map[string]ColumnType{
		"smallint":         SmallInt,
		"integer":          Int,
		"serial":           Serial,
		"bigserial":        BigSerial,
		"real":             Real,
		"double precision": Double,
		"date":             Date,
		"time":             Time,
		"timestamp":        Timestamp,
		"json":             JSON,
		"bytea":            Bytea,
		"inet":             Inet,
		"char(10)":         Char(10),
	}
	for want, ct := range cases {
		if ct.String() != want {
			t.Errorf("got %q want %q", ct.String(), want)
		}
	}
}
