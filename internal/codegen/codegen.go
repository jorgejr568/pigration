// Package codegen renders pigration migration files and the throwaway `go run`
// runner entrypoint, and resolves the migrations import path from go.mod.
package codegen

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

// splitWords splits an identifier-ish string into lowercase words. Any rune that
// is not a letter or digit (spaces, hyphens, underscores, punctuation, quotes,
// semicolons, dots, ...) is a separator and is dropped, so the words can only
// contain safe identifier characters. Words also break on camelCase boundaries.
func splitWords(s string) []string {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// break before an uppercase run boundary (camelCase splitting)
			if unicode.IsUpper(r) && cur.Len() > 0 && i > 0 && !unicode.IsUpper(runes[i-1]) {
				flush()
			}
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return words
}

// SnakeCase converts a raw name to snake_case.
func SnakeCase(s string) string {
	return strings.Join(splitWords(s), "_")
}

// CamelCase converts a raw name to UpperCamelCase.
func CamelCase(s string) string {
	var b strings.Builder
	for _, w := range splitWords(s) {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		b.WriteString(string(r))
	}
	return b.String()
}

// MigrationData is the template payload for a generated migration file.
type MigrationData struct {
	Package  string // package name, e.g. "migrations"
	FuncBase string // CamelCase name + timestamp, e.g. "CreateUsers1719800000"
	Name     string // registered identity, e.g. "1719800000_create_users"
}

var migrationTmpl = template.Must(template.New("migration").Parse(`package {{.Package}}

import (
	"context"

	"github.com/jorgejr568/pigration/migrator"
)

func {{.FuncBase}}Up(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, ` + "`" + `CREATE TABLE example (
	id         bigserial   PRIMARY KEY,
	created_at timestamptz NOT NULL DEFAULT now()
);` + "`" + `)
	return err
}

func {{.FuncBase}}Down(ctx context.Context, tx migrator.Executor) error {
	_, err := tx.Exec(ctx, ` + "`" + `DROP TABLE example;` + "`" + `)
	return err
}

func init() {
	// For operations that cannot run inside a transaction (e.g.
	// CREATE INDEX CONCURRENTLY), add migrator.NonTransactional() as a
	// trailing option:
	//
	//	migrator.Register("{{.Name}}",
	//		{{.FuncBase}}Up, {{.FuncBase}}Down, migrator.NonTransactional())
	migrator.Register("{{.Name}}",
		{{.FuncBase}}Up, {{.FuncBase}}Down)
}
`))

// RenderMigration renders a migration file. rawName is the human name given on
// the CLI; ts is the unix timestamp. It returns the filename and file content.
func RenderMigration(pkg, rawName string, ts int64) (filename string, content string, err error) {
	snake := SnakeCase(rawName)
	if snake == "" {
		return "", "", fmt.Errorf("migration name %q produced an empty identifier", rawName)
	}
	tsStr := strconv.FormatInt(ts, 10)
	// FuncBase must be a valid Go identifier. CamelCase can start with a digit
	// (e.g. name "2fa tokens"); prefix a letter so the generated function names
	// always compile.
	funcBase := CamelCase(rawName)
	if r := []rune(funcBase); len(r) > 0 && !unicode.IsLetter(r[0]) {
		funcBase = "M" + funcBase
	}
	data := MigrationData{
		Package:  pkg,
		FuncBase: funcBase + tsStr,
		Name:     tsStr + "_" + snake,
	}
	var b strings.Builder
	if err := migrationTmpl.Execute(&b, data); err != nil {
		return "", "", fmt.Errorf("rendering migration: %w", err)
	}
	return data.Name + ".go", b.String(), nil
}
