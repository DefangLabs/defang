package gcp

import (
	"regexp"
	"strings"
)

// SafeValue converts a string to a safe value for use in labels.
//
// GCP Label value requirement:
//   - The value can only contain lowercase letters, numeric characters, underscores and dashes.
//   - The value can be at most 63 characters long.
//   - International characters are allowed.
//
// Even though the GCP allow "international characters" in labels, we use a subset for simplicity

// \p{Ll} matches any lowercase letter that has an upper case counterpart
// \p{Lo} matches any letter that does not have an upper case counterpart
// See: https://github.com/google/re2/wiki/Syntax#:~:text=Ll,other%20letter
var safeLabelRE = regexp.MustCompile(`[^\p{Ll}\p{Lo}0-9_-]+`)

func SafeLabelValue(input string) string {
	input = strings.ToLower(input)
	safe := safeLabelRE.ReplaceAllString(input, "-")
	if len(safe) > 63 {
		safe = safe[:63]
	}
	return safe
}
