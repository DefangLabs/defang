package pkg

import (
	"reflect"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetenvBool(t *testing.T) {
	if GetenvBool("FOO") {
		t.Errorf("GetenvBool(FOO) = true, want default false")
	}
	t.Setenv("FOO", "true")
	if !GetenvBool("FOO") {
		t.Errorf("GetenvBool(FOO) = false, want true")
	}
	t.Setenv("FOO", "false")
	if GetenvBool("FOO") {
		t.Errorf("GetenvBool(FOO) = true, want false")
	}
}

func TestIsValidServiceName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"", false},
		{"a", true},
		{"a1", true},
		{"www", true},
		{"fine", true},
		{"x--c", false}, // no consecutive hyphens
		{"foo-bar", true},
		{"foo-bar-123", true},
		{"-foo", false},
		{"foo-", false},
		{"foo_bar", true},
		{"foo bar", false},
		{"foo.bar", false},
		{"Dfnx", true},
		{"more-than-63-characters-are-not-allowed-012345678901234567890123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidServiceName(tt.name); got != tt.want {
				t.Errorf("IsValidServiceName(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsValidSecretName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"", false},
		{"a", true},
		{"A1", true},
		{"1A", false}, // no leading digits
		{"www", true},
		{"fine", true},
		{"x_c", true},
		{"foo_bar", true},
		{"foo_bar_123", true},
		{"_foo", true},
		{"foo_", true},
		{"foo-bar", false}, // no hyphens
		{"foo bar", false}, // no spaces
		{"foo.bar", false}, // no dots
		{"more_than_64_characters_are_not_allowed_0123456789012345678901234", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidSecretName(tt.name); got != tt.want {
				t.Errorf("IsValidSecretName(%v) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSplitByComma(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"one", "a", []string{"a"}},
		{"two", "a,b", []string{"a", "b"}},
		{"three", "a,b,c", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SplitByComma(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SplitByComma(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestRandomID(t *testing.T) {
	var unique = make(map[string]bool)
	for range 100 {
		id := RandomID()
		if unique[id] {
			t.Errorf("RandomID() = %v, want unique ID", id)
		}
		unique[id] = true
		if !IsValidRandomID(id) {
			t.Errorf("RandomID() = %v, want IsValidRandomID true", id)
		}
	}
}

func TestIsValidTime(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		expected bool
	}{
		{"Valid time", time.Now(), true},
		{"Zero time", time.Time{}, false},
		{"From zero Timestamppb", timestamppb.New(time.Time{}).AsTime(), false},
		{"From now Timestamppb", timestamppb.Now().AsTime(), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidTime(tt.time); got != tt.expected {
				t.Errorf("IsValidTime() returned %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestGetCurrentUser(t *testing.T) {
	if got := GetCurrentUser(); got == "" {
		t.Errorf("GetCurrentUser() returned an empty string")
	}

	t.Setenv("USER", "test")
	if got := GetCurrentUser(); got != "test" {
		t.Errorf("GetCurrentUser() returned %v, expected test", got)
	}

	t.Setenv("USER", "")
	t.Setenv("USERNAME", "testx")
	if got := GetCurrentUser(); got != "testx" {
		t.Errorf("GetCurrentUser() returned %v, expected testx", got)
	}

	t.Setenv("USERNAME", "")
	if got := GetCurrentUser(); got == "" {
		t.Errorf("GetCurrentUser() returned an empty string")
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{
			input:    []string{"true"},
			expected: `true`,
		},
		{
			input:    []string{"echo", "hello world"},
			expected: `echo "hello world"`,
		},
		{
			input:    []string{"echo", "hello", "world"},
			expected: `echo hello world`,
		},
		{
			input:    []string{"echo", `hello"world`},
			expected: `echo "hello\"world"`,
		},
		{
			input:    []string{"bash", "-c", "start.sh $PORT"},
			expected: `bash -c "start.sh $PORT"`,
		},
	}

	for _, test := range tests {
		actual := ShellQuote(test.input...)
		if actual != test.expected {
			t.Errorf("Expected `%s` but got: `%s`", test.expected, actual)
		}
	}
}

func TestGetFirstEnv(t *testing.T) {
	tests := []struct {
		name     string
		keys     []string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "No environment variables set",
			keys:     []string{"VAR1", "VAR2", "VAR3"},
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name:     "First variable is set",
			keys:     []string{"VAR1", "VAR2", "VAR3"},
			envVars:  map[string]string{"VAR1": "value1"},
			expected: "value1",
		},
		{
			name:     "Second variable is set",
			keys:     []string{"VAR1", "VAR2", "VAR3"},
			envVars:  map[string]string{"VAR2": "value2"},
			expected: "value2",
		},
		{
			name:     "Multiple variables set, returns first",
			keys:     []string{"VAR1", "VAR2", "VAR3"},
			envVars:  map[string]string{"VAR2": "value2", "VAR3": "value3"},
			expected: "value2",
		},
		{
			name:     "All variables set, returns first",
			keys:     []string{"VAR1", "VAR2", "VAR3"},
			envVars:  map[string]string{"VAR1": "value1", "VAR2": "value2", "VAR3": "value3"},
			expected: "value1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			
			got := GetFirstEnv(tt.keys...)
			if got != tt.expected {
				t.Errorf("GetFirstEnv(%v) = %v, want %v", tt.keys, got, tt.expected)
			}
		})
	}
}
