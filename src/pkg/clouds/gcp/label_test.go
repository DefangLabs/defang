package gcp

import (
	"strings"
	"testing"
)

func TestSafeLabelValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"valid-label", "valid-label"},
		{"with-UpperCase", "with-u\u0307pperc\u0307ase"},
		{"with_special@chars!", "with_special-chars-"},
		{"long_label_" + strings.Repeat("a", 60), "long_label_" + strings.Repeat("a", 52)},
		{"", ""},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := SafeLabelValue(test.input)
			if result != test.expected {
				t.Errorf("expected %q, got %q", test.expected, result)
			}
		})
	}
}

func TestEscapeUnescapeUpperCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"all-lower", "all-lower"},
		{"someUpperCase", "someu\u0307pperc\u0307ase"},
		{"ALL_CAPS", "a\u0307l\u0307l\u0307_c\u0307a\u0307p\u0307s\u0307"},
		{"", ""},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			result := EscapeUpperCase(test.input)
			if result != test.expected {
				t.Errorf("expected %q, got %q", test.expected, result)
			}
			unescaped := UnescapeUpperCase(result)
			if unescaped != test.input {
				t.Errorf("expected unescaped %q, got %q", test.input, unescaped)
			}
		})
	}
}
