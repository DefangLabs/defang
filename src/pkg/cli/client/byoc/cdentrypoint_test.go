package byoc

import (
	"slices"
	"testing"
)

func TestCDEntrypoint(t *testing.T) {
	tests := []struct {
		name     string
		cdImage  string
		expected []string
	}{
		{
			name:     "legacy public CD image",
			cdImage:  "public.ecr.aws/defang-io/cd:public-cd-image-3b4eb980-arm64@sha256:5233e7e84a8a128ba47f36db3ce0eff10ae6242adb5568c7b1e2e0474b38a677",
			expected: NodeCDEntrypoint,
		},
		{
			name:     "legacy nodejs CD image",
			cdImage:  "381492210770.dkr.ecr.us-west-2.amazonaws.com/defang-cd-ecr-public/defang-io/cd:nodejs-cd-image-707883e0-x86_64",
			expected: NodeCDEntrypoint,
		},
		{
			name:     "released Go CD image",
			cdImage:  "public.ecr.aws/defang-io/cd:v2.6.0-aws@sha256:5e7c7a1931fda2ce7def36a1da3daefa1d1221b573d41a3131b299e15543f8a7",
			expected: GoCDEntrypoint,
		},
		{
			name:     "prerelease Go CD image",
			cdImage:  "ghcr.io/defanglabs/cd:2.6.0-alpha-37499b67",
			expected: GoCDEntrypoint,
		},
		{
			name:     "PR-built Go CD image",
			cdImage:  "ghcr.io/defanglabs/cd:pr-423@sha256:01b3184e0d022c3e1c321015e933d91b4d0ddd552e0ab0826c5a9dd57aebe848",
			expected: GoCDEntrypoint,
		},
		{
			name:     "untagged image",
			cdImage:  "public.ecr.aws/defang-io/cd",
			expected: GoCDEntrypoint,
		},
		{
			name:     "registry with port is not mistaken for a tag",
			cdImage:  "localhost:5000/cd:public-cd-image-3b4eb980-arm64",
			expected: NodeCDEntrypoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CDEntrypoint(tt.cdImage); !slices.Equal(got, tt.expected) {
				t.Errorf("CDEntrypoint(%q) = %v, expected %v", tt.cdImage, got, tt.expected)
			}
		})
	}
}

func TestCDEntrypointOverride(t *testing.T) {
	// The override exists for images whose tag says nothing about their flavour.
	t.Setenv("DEFANG_CD_ENTRYPOINT", "/app/cd")

	legacyImage := "public.ecr.aws/defang-io/cd:public-cd-image-3b4eb980-arm64"
	if got := CDEntrypoint(legacyImage); !slices.Equal(got, []string{"/app/cd"}) {
		t.Errorf("CDEntrypoint(%q) = %v, expected the override to win", legacyImage, got)
	}
}
