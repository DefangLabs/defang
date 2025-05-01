package dns

import "testing"

func TestSafeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.project", "example-project"},
		{"EXAMPLE.PROJECT", "example-project"},
		{"example.project.", "example-project-"},
	}

	for _, test := range tests {
		result := SafeLabel(test.input)
		if result != test.expected {
			t.Errorf("SafeLabel(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"example.com.", "example.com"},
	}

	for _, test := range tests {
		result := Normalize(test.input)
		if result != test.expected {
			t.Errorf("Normalize(%q) = %q; want %q", test.input, result, test.expected)
		}
	}
}
