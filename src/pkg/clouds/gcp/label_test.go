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
		{"with-UpperCase", "with-uppercase"},
		{"with_special@chars!", "with_special-chars-"},
		{"long_label_" + strings.Repeat("a", 60), "long_label_" + strings.Repeat("a", 52)},
		{"ṠervicėWithUnicode", "ṡervicėwithunicode"},
		{"服务，标点。", "服务-标点-"},
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
