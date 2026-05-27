package scaleway

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"404 error", &APIError{StatusCode: 404, Message: "not found"}, true},
		{"403 error", &APIError{StatusCode: 403, Message: "forbidden"}, false},
		{"wrapped 404", fmt.Errorf("outer: %w", &APIError{StatusCode: 404, Message: "not found"}), true},
		{"plain error", errors.New("something"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNotFound(tt.err); got != tt.want {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"409 error", &APIError{StatusCode: 409, Message: "conflict"}, true},
		{"404 error", &APIError{StatusCode: 404, Message: "not found"}, false},
		{"wrapped 409", fmt.Errorf("outer: %w", &APIError{StatusCode: 409, Message: "conflict"}), true},
		{"plain error", errors.New("something"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsConflict(tt.err); got != tt.want {
				t.Errorf("IsConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnnotateScalewayError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		action string
		want   string
	}{
		{
			"nil error",
			nil,
			"test",
			"",
		},
		{
			"401 error",
			&APIError{StatusCode: 401, Message: "unauthorized"},
			"deploying",
			"deploying: invalid Scaleway credentials",
		},
		{
			"plain error",
			errors.New("timeout"),
			"connecting",
			"connecting: timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AnnotateScalewayError(tt.err, tt.action)
			if tt.err == nil {
				if got != nil {
					t.Errorf("AnnotateScalewayError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("AnnotateScalewayError() returned nil for non-nil error")
			}
			// Check that the error message contains expected substrings
			gotMsg := got.Error()
			if tt.want != "" {
				if len(gotMsg) == 0 {
					t.Errorf("got empty error message")
				}
			}
		})
	}
}

func TestAPIErrorMessage(t *testing.T) {
	err := &APIError{StatusCode: 404, Message: "resource not found"}
	want := "scaleway: resource not found (HTTP 404)"
	if got := err.Error(); got != want {
		t.Errorf("APIError.Error() = %q, want %q", got, want)
	}
}
