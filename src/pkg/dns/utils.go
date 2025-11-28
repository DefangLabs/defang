package dns

import "strings"

func SafeLabel(fqn string) string {
	return strings.ReplaceAll(strings.ToLower(fqn), ".", "-")
}

func Normalize(domain string) string {
	return strings.ToLower(strings.TrimSuffix(domain, "."))
}
