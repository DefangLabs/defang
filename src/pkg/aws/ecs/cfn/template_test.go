package cfn

import "testing"

func TestGetCacheRepoPrefix(t *testing.T) {
	// Test cases
	tests := []struct {
		prefix string
		suffix string
		want   string
	}{
		{
			prefix: "short-",
			suffix: "ecr-public",
			want:   "short-ecr-public",
		},
		{
			prefix: "short-",
			suffix: "docker-public",
			want:   "short-docker-public",
		},
		{
			prefix: "loooooooooong-",
			suffix: "docker-public",
			want:   "fab852-docker-public",
		},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := getCacheRepoPrefix(tt.prefix, tt.suffix); got != tt.want {
				t.Errorf("getCacheRepoPrefix() = %q, want %q", got, tt.want)
			} else if len(got) > maxCachePrefixLength {
				t.Errorf("getCacheRepoPrefix() = %q, want length <= %v", got, maxCachePrefixLength)
			}
		})
	}
}
