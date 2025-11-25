package crun

import (
	"testing"
)

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

func TestParseEnvLine(t *testing.T) {
	const FROMENV = "blah"
	t.Setenv("FROMENV", FROMENV)

	testCases := []struct {
		line  string
		key   string
		value string
	}{
		{}, // empty line
		{
			line: "# comment",
		},
		{
			line: "   # comment with leading spaces",
		},
		{
			line:  "key=value",
			key:   "key",
			value: "value",
		},
		{
			line:  "key = value", // docker errors on this, but we don't
			key:   "key",
			value: " value",
		},
		{
			line:  " key=value ",
			key:   "key",
			value: "value ",
		},
		{
			line:  "FROMENV",
			key:   "FROMENV",
			value: FROMENV,
		},
		{
			line: "EMPTY=",
			key:  "EMPTY",
		},
		{
			line: "NONEXISTENT",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.line, func(t *testing.T) {
			key, value := ParseEnvLine(tC.line)
			if key != tC.key || value != tC.value {
				t.Errorf("expected %v=%q, got %v=%q", tC.key, tC.value, key, value)
			}
		})
	}
}

func TestParseEnvFile(t *testing.T) {
	env := make(map[string]string)
	parseEnvFile(`# comment
key=value
key2= value2
# comment with leading space
key3=value3 # comment at end
key4=crlf`+"\r\n", env)
	if len(env) != 4 {
		t.Errorf("expected 4 env vars, got %v", len(env))
	}
	if env["key"] != "value" {
		t.Errorf("expected 'value', got %q", env["key"])
	}
	if env["key2"] != " value2" {
		t.Errorf("expected ' value2', got %q", env["key2"])
	}
	if env["key3"] != "value3 # comment at end" {
		t.Errorf("expected 'value3 # comment at end', got %q", env["key3"])
	}
	if env["key4"] != "crlf" {
		t.Errorf("expected 'crlf', got %q", env["key4"])
	}
}
