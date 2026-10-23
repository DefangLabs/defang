package cli

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"strings"
	"unicode"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

// CreateRandomConfigValue generates a value suitable for the selected provider.
func CreateRandomConfigValue(provider client.ProviderID) string {
	return createRandomConfigValue(provider == client.ProviderScaleway)
}

// Some managed databases require symbols as well as upper/lowercase and digits.
// Rejection sampling preserves randomness without adding a predictable prefix.
func createRandomConfigValue(requireSymbols bool) string {
	symbols := regexp.MustCompile("[^a-zA-Z0-9]")
	for {
		key := make([]byte, 32)
		rand.Read(key)
		value := base64.RawURLEncoding.EncodeToString(key)
		if !requireSymbols {
			return symbols.ReplaceAllString(value, "")
		}
		if symbols.MatchString(value) && strings.ContainsFunc(value, unicode.IsUpper) &&
			strings.ContainsFunc(value, unicode.IsLower) && strings.ContainsFunc(value, unicode.IsDigit) {
			return value
		}
	}
}
