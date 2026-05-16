package cli

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

// CreateRandomConfigValue generates a random config value appropriate for the
// given provider. Some providers (e.g. Scaleway) have strict password policies
// for managed databases that require uppercase, lowercase, digits, and special
// characters.
func CreateRandomConfigValue(provider client.ProviderID) string {
	switch provider {
	case client.ProviderScaleway:
		return createScalewayCompatibleValue()
	default:
		return createDefaultRandomValue()
	}
}

// createDefaultRandomValue generates a URL-safe alphanumeric random string.
func createDefaultRandomValue() string {
	key := make([]byte, 32)
	rand.Read(key)
	str := base64.StdEncoding.EncodeToString(key)
	re := regexp.MustCompile("[+/=]")
	str = re.ReplaceAllString(str, "")
	return str
}

// createScalewayCompatibleValue generates a random value that satisfies
// Scaleway Managed Database password requirements: 8-128 characters with
// uppercase, lowercase, digit, and special character.
func createScalewayCompatibleValue() string {
	key := make([]byte, 32)
	rand.Read(key)
	str := base64.URLEncoding.EncodeToString(key) // uses - and _ instead of + and /
	// Remove padding
	re := regexp.MustCompile("[=]")
	str = re.ReplaceAllString(str, "")
	// Ensure at least one of each required character class by prepending a
	// fixed prefix. The rest of the string provides enough entropy.
	return "Df1!" + str
}
