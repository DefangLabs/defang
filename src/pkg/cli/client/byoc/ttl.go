package byoc

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// TTLEnvVar is the environment variable the CD program reads into its
// defang:ttl stack config to schedule the stack's self-destruct (a
// "defang cd down" at deploy time + TTL).
const TTLEnvVar = "DEFANG_TTL"

// ValidateTTL syntax-checks a TTL value for fast feedback before a CD task is
// started. It accepts exactly the syntax the CD program parses (parseTTL in
// pulumi-defang's cd/program/ttl.go): "", "never" and "0" mean no
// self-destruct; any other value is a Go duration with an optional whole-days
// prefix ("12h", "7d", "7d12h"). The min/max bounds (1h to 10 years) are
// deliberately NOT checked here: the CD enforces them authoritatively, and
// duplicating the numbers on both sides would let the rules drift apart.
func ValidateTTL(value string) error {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "", "never", "0":
		return nil
	}
	if i := strings.IndexByte(s, 'd'); i > 0 {
		if n, err := strconv.Atoi(s[:i]); err != nil || n < 0 {
			return fmt.Errorf("invalid TTL %q: days prefix must be a whole number", value)
		}
		s = s[i+1:]
	}
	if s != "" {
		if _, err := time.ParseDuration(s); err != nil {
			return fmt.Errorf(`invalid TTL %q: use a duration like "12h" or "7d", or "never"`, value)
		}
	}
	return nil
}

// AddTTLEnv copies DEFANG_TTL from the CLI's environment — set by the --ttl
// flag or by a DEFANG_TTL stack-file variable — into a CD run's environment.
// The key is only set when a TTL was given: to the CD, absent and empty both
// mean "no self-destruct".
func AddTTLEnv(env map[string]string) {
	if ttl := os.Getenv(TTLEnvVar); ttl != "" {
		env[TTLEnvVar] = ttl
	}
}
