package pkg

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestGetenvBool(t *testing.T) {
	if GetenvBool("FOO") {
		t.Errorf("GetenvBool(FOO) = true, want default false")
	}
	os.Setenv("FOO", "true")
	if !GetenvBool("FOO") {
		t.Errorf("GetenvBool(FOO) = false, want true")
	}
	os.Setenv("FOO", "false")
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
		{"foo_bar", false},
		{"foo bar", false},
		{"foo.bar", false},
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

func TestOneOrList(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", `[]`, []string{}},
		{"string", `"a"`, []string{"a"}},
		{"one", `["a"]`, []string{"a"}},
		{"two", `["a","b"]`, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got OneOrList
			if err := json.Unmarshal([]byte(tt.in), &got); err != nil || !reflect.DeepEqual([]string(got), tt.want) {
				t.Errorf("OneOrList(%v) = %v, want %v: %v", tt.in, got, tt.want, err)
			}
		})
	}
}

func TestRandomID(t *testing.T) {
	var unique = make(map[string]bool)
	for i := 0; i < 100; i++ {
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
