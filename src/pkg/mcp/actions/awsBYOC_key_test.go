package actions

import (
	"testing"
)

func TestIsValidAWSKey_ValidKeys(t *testing.T) {
	validKeys := []string{
		"AKIA12345678901234",
		"AIDA12345678901234",
		"ASIA12345678901234",
		"APKA12345678901234",
		"AROA12345678901234",
		"ASCA12345678901234",
	}
	for _, key := range validKeys {
		if !IsValidAWSKey(key) {
			t.Errorf("expected key %q to be valid", key)
		}
	}
}

func TestIsValidAWSKey_InvalidKeys(t *testing.T) {
	invalidKeys := []string{
		"",                   // empty
		"AKIA1234",           // too short
		"AKIA",               // too short
		"AKIA1234567890",     // too short
		"AKI12345678901234",  // prefix too short
		"ZZZZ12345678901234", // invalid prefix
	}
	for _, key := range invalidKeys {
		if IsValidAWSKey(key) {
			t.Errorf("expected key %q to be invalid", key)
		}
	}
}
