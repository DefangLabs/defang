package compose

import "testing"

func TestIsStatefulImage(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected bool
	}{
		{
			name:     "Stateful image",
			image:    "redis",
			expected: true,
		},
		{
			name:     "Stateful image with repo",
			image:    "library/redis",
			expected: true,
		},
		{
			name:     "Stateful image with tag",
			image:    "redis:6.0",
			expected: true,
		},
		{
			name:     "Stateful image with registry",
			image:    "docker.io/redis",
			expected: true,
		},
		{
			name:     "Stateless image",
			image:    "alpine:latest",
			expected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isStatefulImage(tt.image); got != tt.expected {
				t.Errorf("isStatefulImage() = %v, want %v", got, tt.expected)
			}
		})
	}
}
