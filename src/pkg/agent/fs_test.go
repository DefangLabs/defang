package agent

import (
	"testing"
)

func TestIsSafePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{
			name:     "Empty path",
			path:     "",
			expected: true,
		},
		{
			name:     "Simple relative path",
			path:     "file.txt",
			expected: true,
		},
		{
			name:     "Relative path with directory",
			path:     "dir/file.txt",
			expected: true,
		},
		{
			name:     "Current directory",
			path:     "./file.txt",
			expected: true,
		},
		{
			name:     "Absolute path",
			path:     "/etc/passwd",
			expected: false,
		},
		{
			name:     "Parent directory traversal",
			path:     "../file.txt",
			expected: false,
		},
		{
			name:     "Nested parent directory traversal",
			path:     "foo/../../file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSafePath(tt.path)
			if got != tt.expected {
				t.Errorf("isSafePath(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}
