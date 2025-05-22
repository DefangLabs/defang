package money

import (
	"testing"
)

func TestMoney(t *testing.T) {
	tests := []struct {
		amount   float64
		currency string
		expected string
	}{
		{1.23, "USD", "$1.23"},
		{1.23, "GBP", "£1.23"},
		{0, "USD", "$0.00"},
		{-1.23, "USD", "-$1.23"},
	}

	for _, test := range tests {
		m := NewMoney(test.amount, test.currency)
		if m.String() != test.expected {
			t.Errorf("Expected %s but got %s", test.expected, m.String())
		}
	}
}
