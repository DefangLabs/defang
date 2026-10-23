package cli

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"
)

// SymbolPolicy specifies the permitted character classes for a generated value.
type SymbolPolicy uint8

const (
	SymbolsExcluded SymbolPolicy = iota
	SymbolsRequired
)

// CreateRandomConfigValue generates a random value with the requested symbol policy.
// Rejection sampling satisfies character requirements without adding a fixed prefix.
func CreateRandomConfigValue(policy SymbolPolicy) string {
	symbols := regexp.MustCompile("[^a-zA-Z0-9]")
	for {
		key := make([]byte, 32)
		rand.Read(key)
		value := base64.RawURLEncoding.EncodeToString(key)
		if policy == SymbolsExcluded {
			return symbols.ReplaceAllString(value, "")
		}
		if symbols.MatchString(value) && strings.ContainsFunc(value, unicode.IsUpper) &&
			strings.ContainsFunc(value, unicode.IsLower) && strings.ContainsFunc(value, unicode.IsDigit) {
			return value
		}
	}
}
