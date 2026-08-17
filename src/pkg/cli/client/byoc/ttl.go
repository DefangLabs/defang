package byoc

import (
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/timeutils"
)

// ParseTTL normalizes a deployment time-to-live for the CD program, which
// reads it from the DEFANG_TTL env var of the CD run into its defang:ttl
// stack config (see parseTTL in pulumi-defang's cd/program/ttl.go) to
// schedule the stack's self-destruct. Accepted inputs:
//   - "", "0" and "never": no self-destruct
//   - a Go duration with an optional whole-days prefix ("12h", "7d", "7d12h"):
//     passed through unchanged, the CD parses the same syntax
//   - a timestamp, in the same formats as `logs --since/--until`
//     (RFC3339, unix seconds/millis): translated to the duration from now
//     until that moment, because the CD only accepts durations
//
// Only the syntax (and sign) is checked here, for fast feedback before the
// CD task starts; the 1h..10y min/max bounds are deliberately NOT duplicated
// on the CLI side — the CD enforces them authoritatively, so the rules
// cannot drift.
func ParseTTL(value string, now time.Time) (string, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "", "never", "0":
		return s, nil
	}
	if dur, err := timeutils.ParseDuration(s); err == nil {
		if dur <= 0 {
			return "", fmt.Errorf(`invalid TTL %q: must be positive (use "never" to disable)`, value)
		}
		return s, nil
	}
	// Not a duration: try the timestamp formats shared with `logs --since/--until`.
	ts, err := timeutils.ParseTimeOrDuration(value, now)
	if err != nil {
		return "", fmt.Errorf(`invalid TTL %q: use a duration like "12h" or "7d", a timestamp, or "never"`, value)
	}
	ttl := ts.Sub(now)
	if ttl <= 0 {
		return "", fmt.Errorf("invalid TTL %q: timestamp is in the past", value)
	}
	return ttl.String(), nil
}
