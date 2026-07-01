package migrator

import "errors"

// ErrFreshNotAllowed is returned by Fresh when the caller has not opted in via
// AllowFresh() (or the CLI's PIGRATION_ALLOW_FRESH=1 / fresh.allow config).
var ErrFreshNotAllowed = errors.New("fresh not allowed: set AllowFresh() / PIGRATION_ALLOW_FRESH=1")

// ErrLocked is returned when the migration advisory lock could not be acquired
// because another migration run holds it.
var ErrLocked = errors.New("another migration is in progress")
