package actions

// Check if the provided AWS access key ID is valid
// https://medium.com/@TalBeerySec/a-short-note-on-aws-key-id-f88cc4317489
func IsValidAWSKey(key string) bool {
	// Define accepted AWS access key prefixes
	acceptedPrefixes := map[string]bool{
		"ABIA": true,
		"ACCA": true,
		"AGPA": true,
		"AIDA": true,
		"AKPA": true,
		"AKIA": true,
		"ANPA": true,
		"ANVA": true,
		"APKA": true,
		"AROA": true,
		"ASCA": true,
		"ASIA": true,
	}

	if len(key) < 16 {
		return false
	}

	prefix := key[:4]
	_, ok := acceptedPrefixes[prefix]
	return ok
}
