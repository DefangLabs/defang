package cmd

import "testing"

func TestParseMemory(t *testing.T) {
	testCases := []struct {
		memory   string
		expected uint64
	}{
		{
			memory:   "5000000",
			expected: 5000000,
		},
		{
			memory:   "6MB",
			expected: 6000000,
		},
		{
			memory:   "7MiB",
			expected: 7340032,
		},
		{
			memory:   "8k",
			expected: 8192,
		},
		{
			memory:   "9gIb",
			expected: 9663676416,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.memory, func(t *testing.T) {
			actual := ParseMemory(tC.memory)
			if actual != tC.expected {
				t.Errorf("expected %v, got %v", tC.expected, actual)
			}
		})
	}
}
