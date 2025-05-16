package money

import (
	"fmt"
	"math"
	"strings"

	google "github.com/DefangLabs/defang/src/protos/google/type"
)

type Money google.Money

// Currency symbol map (expand as needed)
var currencySymbols = map[string]string{
	"USD": "$",
	"EUR": "€",
	"GBP": "£",
	"JPY": "¥",
	// Add more as needed
}

func NewMoney(amount float64, currency string) *Money {
	units := int64(amount)
	nanos := int32(math.Round((amount - float64(units)) * 1e9))

	// Fix signs: if nanos and units mismatch, adjust accordingly
	if amount < 0 && nanos > 0 {
		units--
		nanos = nanos - 1e9
	}
	if amount > 0 && nanos < 0 {
		units++
		nanos = nanos + 1e9
	}

	return &Money{
		CurrencyCode: currency,
		Units:        units,
		Nanos:        nanos,
	}
}

func (m *Money) String() string {
	symbol, ok := currencySymbols[strings.ToUpper(m.CurrencyCode)]
	if !ok {
		symbol = m.CurrencyCode + " "
	}

	// Combine units and nanos
	total := float64(m.Units) + float64(m.Nanos)/1e9
	if total < 0 {
		return fmt.Sprintf("-%s%.2f", symbol, -total)
	}
	return fmt.Sprintf("%s%.2f", symbol, total)
}
