package gcp

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// SafeValue converts a string to a safe value for use in labels.
//
// GCP Label value requirement:
//   - The value can only contain lowercase letters, numeric characters, underscores and dashes.
//   - The value can be at most 63 characters long.
//   - International characters are allowed.
//
// Even though the GCP allow "international characters" in labels, we use a subset for simplicity
var safeLabelRE = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func SafeLabelValue(input string) string {
	safe := safeLabelRE.ReplaceAllString(input, "-")
	if len(safe) > 63 {
		safe = safe[:63]
	}
	safe = EscapeUpperCase(safe)
	return safe
}

const CombiningDotAbove = '\u0307' // U+0307 Combining Dot Above

func EscapeUpperCase(input string) string {
	// Fast path: if there are no uppercase letters, return the input as is
	if strings.IndexFunc(input, func(r rune) bool {
		return r >= 'A' && r <= 'Z'
	}) == -1 {
		return input
	}

	var buf strings.Builder
	for _, r := range input {
		if r >= 'A' && r <= 'Z' {
			buf.WriteRune(r + 32)
			// Hack: Use unicode combining characters to indicate this was an uppercase letter
			buf.WriteRune(CombiningDotAbove)
		} else {
			buf.WriteRune(r)
		}
	}

	return norm.NFC.String(buf.String())
}

func UnescapeUpperCase(input string) string {
	input = norm.NFD.String(input)

	var output []rune
	runes := []rune(input)
	for _, r := range runes {
		if r == CombiningDotAbove {
			if l := len(output); l > 0 && output[l-1] >= 'a' && output[l-1] <= 'z' {
				output[l-1] -= 32 // Convert the last character to uppercase
			}
			continue
		}
		output = append(output, r)
	}

	return string(output)
}
