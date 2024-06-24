package compose

import (
	"regexp"
	"strings"
)

var (
	nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

func NormalizeServiceName(s string) string {
	// TODO: replace with the code from compose-go
	return nonAlphanumeric.ReplaceAllLiteralString(strings.ToLower(s), "-")
}
