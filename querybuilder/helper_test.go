package querybuilder

import "strings"

// norm collapses runs of whitespace into a single space and trims, so SQL
// assertions are robust to formatting differences.
func norm(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
