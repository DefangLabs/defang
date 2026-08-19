package byoc

import (
	"fmt"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/timeutils"
)

// minTTL is the shortest TTL the CLI accepts. The CD's self-destruct trigger
// clock starts at deploy time, but resources can take 10+ minutes to finish
// provisioning after that — a shorter TTL could tear the stack down while (or
// right after) it comes up. This floor is enforced here, in the CLI, and
// deliberately NOT in the CD (see parseTTL in pulumi-defang's
// cd/program/ttl.go): the CD accepts any positive TTL so tests and manual CD
// invocations that bypass this CLI check can use shorter durations to verify
// the self-destruct trigger without waiting an hour.
const minTTL = time.Hour

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
// The minTTL floor is enforced here; the CD's maxTTL remains the sole,
// authoritative guard against typo'd far-future dates, so that rule cannot
// drift between the two sides.
func ParseTTL(value string, now time.Time) (string, error) {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "", "never", "0":
		return s, nil
	}
	if dur, err := timeutils.ParseDuration(s); err == nil {
		if dur < minTTL {
			return "", fmt.Errorf(`invalid TTL %q: must be at least %s (use "never" to disable)`, value, minTTL)
		}
		return s, nil
	}
	// Not a duration: try the timestamp formats shared with `logs --since/--until`.
	ts, err := timeutils.ParseTimeOrDuration(value, now)
	if err != nil {
		return "", fmt.Errorf(`invalid TTL %q: use a duration like "12h" or "7d", a timestamp, or "never"`, value)
	}
	ttl := ts.Sub(now)
	if ttl < minTTL {
		return "", fmt.Errorf(`invalid TTL %q: must be at least %s from now`, value, minTTL)
	}
	return ttl.String(), nil
}
