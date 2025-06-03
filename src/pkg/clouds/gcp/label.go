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

var safeLabelRE = regexp.MustCompile(`[^a-z0-9_-]+`)

func SafeLabelValue(input string) string {
	input = strings.ToLower(input)
	safe := safeLabelRE.ReplaceAllString(input, "-")
	if len(safe) > 63 {
		safe = safe[:63]
	}
	return safe
}
