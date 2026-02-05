package term

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type TestStruct struct {
	Name    string
	Age     int
	Email   string
	Active  bool
	Score   float64
	Missing string // Field that won't be shown
}

func TestTable_JSONMode_SingleStruct(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.SetJSON(true)

	data := TestStruct{
		Name:   "Alice",
		Age:    30,
		Email:  "alice@example.com",
		Active: true,
		Score:  95.5,
	}

	err := term.Table(data, "Name", "Age", "Email")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	// Verify JSON output
	var result TestStruct
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}

	if result.Name != data.Name || result.Age != data.Age || result.Email != data.Email {
		t.Errorf("JSON output = %+v, want %+v", result, data)
	}
}

func TestTable_JSONMode_SliceOfStructs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.SetJSON(true)

	data := []TestStruct{
		{Name: "Alice", Age: 30, Email: "alice@example.com"},
		{Name: "Bob", Age: 25, Email: "bob@example.com"},
	}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	// Verify JSON output
	var result []TestStruct
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}

	if len(result) != len(data) {
		t.Errorf("JSON output length = %d, want %d", len(result), len(data))
	}
}

func TestTable_JSONMode_SliceOfInterfaces(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.SetJSON(true)

	data := []any{
		TestStruct{Name: "Alice", Age: 30},
		TestStruct{Name: "Bob", Age: 25},
	}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	// Verify it outputs valid JSON
	var result []any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON output: %v", err)
	}

	if len(result) != len(data) {
		t.Errorf("JSON output length = %d, want %d", len(result), len(data))
	}
}

func TestTable_TableMode_SingleStruct(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := TestStruct{
		Name:  "Alice",
		Age:   30,
		Email: "alice@example.com",
	}

	err := term.Table(data, "Name", "Age", "Email")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have 2 lines: header + 1 data row
	if len(lines) != 2 {
		t.Errorf("Expected 2 lines, got %d\nOutput:\n%s", len(lines), output)
	}

	// Check header
	if !strings.Contains(lines[0], "NAME") || !strings.Contains(lines[0], "AGE") || !strings.Contains(lines[0], "EMAIL") {
		t.Errorf("Header line doesn't contain expected columns: %s", lines[0])
	}

	// Check data row
	if !strings.Contains(lines[1], "Alice") || !strings.Contains(lines[1], "30") || !strings.Contains(lines[1], "alice@example.com") {
		t.Errorf("Data row doesn't contain expected values: %s", lines[1])
	}
}

func TestTable_TableMode_SliceOfStructs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := []TestStruct{
		{Name: "Alice", Age: 30, Email: "alice@example.com"},
		{Name: "Bob", Age: 25, Email: "bob@example.com"},
		{Name: "Charlie", Age: 35, Email: "charlie@example.com"},
	}

	err := term.Table(data, "Name", "Age", "Email")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have 4 lines: header + 3 data rows
	if len(lines) != 4 {
		t.Errorf("Expected 4 lines, got %d\nOutput:\n%s", len(lines), output)
	}

	// Check all names appear in output
	for _, item := range data {
		if !strings.Contains(output, item.Name) {
			t.Errorf("Output doesn't contain name: %s", item.Name)
		}
	}
}

func TestTable_TableMode_SliceOfInterfaces(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := []any{
		TestStruct{Name: "Alice", Age: 30},
		TestStruct{Name: "Bob", Age: 25},
	}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should have 3 lines: header + 2 data rows
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d\nOutput:\n%s", len(lines), output)
	}

	if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
		t.Errorf("Output doesn't contain expected names")
	}
}

func TestTable_TableMode_SliceOfPointers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	alice := TestStruct{Name: "Alice", Age: 30}
	bob := TestStruct{Name: "Bob", Age: 25}
	data := []*TestStruct{&alice, &bob}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
		t.Errorf("Output doesn't contain expected names")
	}
}

func TestTable_TableMode_MissingFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := TestStruct{
		Name: "Alice",
		Age:  30,
	}

	// Request a field that doesn't exist
	err := term.Table(data, "Name", "NonExistentField", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	// Should contain N/A for missing field
	if !strings.Contains(output, "N/A") {
		t.Errorf("Output doesn't contain N/A for missing field:\n%s", output)
	}
}

func TestTable_TableMode_ZeroValues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := []TestStruct{
		{Name: "Alice", Age: 30, Email: "alice@example.com"},
		{Name: "Bob", Age: 0, Email: ""}, // Zero values
	}

	err := term.Table(data, "Name", "Age", "Email")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	// Both rows should be present
	if !strings.Contains(output, "Alice") || !strings.Contains(output, "Bob") {
		t.Errorf("Output doesn't contain expected names")
	}
}

func TestTable_TableMode_WithColor(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(true)

	data := TestStruct{
		Name: "Alice",
		Age:  30,
	}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()

	// When color is enabled, output should contain ANSI escape sequences
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("Expected ANSI color codes in output when color is enabled")
	}
}

func TestTable_TableMode_EmptySlice(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := []TestStruct{}

	err := term.Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should only have header line
	if len(lines) != 1 {
		t.Errorf("Expected 1 line (header only), got %d\nOutput:\n%s", len(lines), output)
	}
}

func TestTable_TableMode_BooleanValues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	data := []TestStruct{
		{Name: "Alice", Active: true},
		{Name: "Bob", Active: false},
	}

	err := term.Table(data, "Name", "Active")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()

	// Should contain boolean values
	if !strings.Contains(output, "true") {
		t.Errorf("Output doesn't contain 'true' for Active field")
	}

	// False is a zero value, so it should be empty/whitespace
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}
}

func TestTable_TableMode_Nil(t *testing.T) {
	var stdout, stderr bytes.Buffer
	term := NewTerm(os.Stdin, &stdout, &stderr)
	term.ForceColor(false)

	err := term.Table(nil, "Name", "Score")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()

	// Should contain float values
	if !strings.Contains(output, "NAME") || !strings.Contains(output, "SCORE") {
		t.Errorf("Output doesn't contain expected headers for nil input")
	}
	if strings.Count(output, "\n") != 1 {
		t.Errorf("Expected only header line for nil input, got:\n%s", output)
	}
}

func TestTable_GlobalFunction(t *testing.T) {
	defaultTerm := DefaultTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})

	var stdout, stderr bytes.Buffer
	DefaultTerm = NewTerm(os.Stdin, &stdout, &stderr)
	DefaultTerm.ForceColor(false)

	data := TestStruct{Name: "Alice", Age: 30}

	err := Table(data, "Name", "Age")
	if err != nil {
		t.Fatalf("Table() error = %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Alice") || !strings.Contains(output, "30") {
		t.Errorf("Global Table() function doesn't work correctly")
	}
}
