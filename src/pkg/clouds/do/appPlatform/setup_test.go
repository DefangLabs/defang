package appPlatform

import "testing"

func TestShellQuote(t *testing.T) {
	// Given
	tests := []struct {
		input    []string
		expected string
	}{
		{
			input:    []string{"true"},
			expected: `"true"`,
		},
		{
			input:    []string{"echo", "hello world"},
			expected: `"echo" "hello world"`,
		},
		{
			input:    []string{"echo", "hello", "world"},
			expected: `"echo" "hello" "world"`,
		},
		{
			input:    []string{"echo", `hello"world`},
			expected: `"echo" "hello\"world"`,
		},
	}

	for _, test := range tests {
		actual := shellQuote(test.input...)
		if actual != test.expected {
			t.Errorf("Expected `%s` but got: `%s`", test.expected, actual)
		}
	}
}
