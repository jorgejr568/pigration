package codegen

import "strconv"

// PIGRATION_* environment variables form the contract between the pigration CLI
// (producer) and the generated `go run` runner (consumer). They are defined here
// once; the CLI imports these constants directly, and the runner template
// interpolates their names at render time so the two sides can never drift.
//
// The protocol is append-only and optional-by-default: unset vars fall back to
// runner defaults, so a newer CLI may emit vars an older runner ignores.
const (
	EnvDSN        = "PIGRATION_DSN"
	EnvTable      = "PIGRATION_TABLE"
	EnvCmd        = "PIGRATION_CMD"
	EnvSteps      = "PIGRATION_STEPS"
	EnvBatch      = "PIGRATION_BATCH"
	EnvAllowFresh = "PIGRATION_ALLOW_FRESH"
)

// RunnerEnv is the typed payload the CLI forwards to the generated runner. Its
// Environ method renders the PIGRATION_* entries, omitting zero-valued fields in
// exactly one place.
type RunnerEnv struct {
	Cmd        string
	Steps      int
	Batch      int
	AllowFresh bool
}

// Environ appends the PIGRATION_* entries for e (plus DSN and table) to base and
// returns the result. base is normally os.Environ(), preserving the toolchain
// env for `go run` and any shell-exported PIGRATION_* passthrough. Steps and
// Batch are emitted only when > 0, matching the CLI's historical guards.
func (e RunnerEnv) Environ(base []string, dsn, table string) []string {
	env := append(base,
		EnvDSN+"="+dsn,
		EnvTable+"="+table,
		EnvCmd+"="+e.Cmd,
	)
	if e.Steps > 0 {
		env = append(env, EnvSteps+"="+strconv.Itoa(e.Steps))
	}
	if e.Batch > 0 {
		env = append(env, EnvBatch+"="+strconv.Itoa(e.Batch))
	}
	if e.AllowFresh {
		env = append(env, EnvAllowFresh+"=1")
	}
	return env
}
