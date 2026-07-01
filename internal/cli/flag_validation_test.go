package cli

import (
	"strings"
	"testing"
)

// Regression: an explicit --batch 0 (or negative) used to be omitted from the
// runner env entirely, silently falling through to the default latest-batch
// rollback — a destructive action the user did not ask for. Explicitly-set
// non-positive --batch/--steps must refuse loudly before anything runs.
func TestExplicitNonPositiveFlagsRefuse(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := runCmd("init"); err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"rollback", "--batch", "0"},
		{"rollback", "--batch", "-2"},
		{"rollback", "--steps", "0"},
		{"migrate", "--steps", "0"},
	}
	for _, args := range cases {
		err := runCmd(args...)
		if err == nil {
			t.Fatalf("%v: expected refusal, got success", args)
		}
		if !strings.Contains(err.Error(), "must be >= 1") {
			t.Fatalf("%v: unhelpful error: %v", args, err)
		}
	}

	// Unset flags keep the default behavior: these must NOT hit flag validation.
	// (They fail later at the go-run stage in this bare temp dir, which is fine —
	// the error must just not be the flag refusal.)
	if err := runCmd("rollback"); err != nil && strings.Contains(err.Error(), "must be >= 1") {
		t.Fatalf("default rollback wrongly hit flag validation: %v", err)
	}
}
