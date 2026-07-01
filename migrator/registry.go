package migrator

import (
	"fmt"
	"sort"
)

// migration is a single registered migration entry.
type migration struct {
	name             string
	up, down         MigrationFunc
	nonTransactional bool
}

// RegisterOption configures a migration at registration time.
type RegisterOption func(*migration)

// NonTransactional marks a migration to run WITHOUT a wrapping transaction, for
// operations that cannot run inside one (CREATE INDEX CONCURRENTLY,
// ALTER TYPE ... ADD VALUE, VACUUM, etc.).
func NonTransactional() RegisterOption {
	return func(m *migration) { m.nonTransactional = true }
}

// registry is the package-level ordered list of registered migrations.
var registry []migration

// Register adds a migration to the package registry. It is normally called from
// an init() function in a generated migration file. Name must be of the form
// "<unixTimestamp>_<snake_name>".
func Register(name string, up, down MigrationFunc, opts ...RegisterOption) {
	m := migration{name: name, up: up, down: down}
	for _, opt := range opts {
		opt(&m)
	}
	registry = append(registry, m)
}

// Registered returns a copy of the registered migrations sorted by name.
func Registered() []migration {
	out := make([]migration, len(registry))
	copy(out, registry)
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// resetRegistry clears the registry (test helper).
func resetRegistry() { registry = nil }

// validateRegistry returns an error if any migration name is registered more
// than once.
func validateRegistry() error {
	seen := make(map[string]struct{}, len(registry))
	for _, m := range registry {
		if _, dup := seen[m.name]; dup {
			return fmt.Errorf("duplicate migration name registered: %q", m.name)
		}
		seen[m.name] = struct{}{}
	}
	return nil
}
