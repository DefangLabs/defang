package aws

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
			expected: nodeCDEntrypoint,
		},
		{
			name:     "legacy nodejs CD image",
			cdImage:  "381492210770.dkr.ecr.us-west-2.amazonaws.com/defang-cd-ecr-public/defang-io/cd:nodejs-cd-image-707883e0-x86_64",
			expected: nodeCDEntrypoint,
		},
		{
			name:     "released Go CD image",
			cdImage:  "public.ecr.aws/defang-io/cd:v2.6.0-aws@sha256:5e7c7a1931fda2ce7def36a1da3daefa1d1221b573d41a3131b299e15543f8a7",
			expected: goCDEntrypoint,
		},
		{
			name:     "prerelease Go CD image",
			cdImage:  "ghcr.io/defanglabs/cd:2.6.0-alpha-37499b67",
			expected: goCDEntrypoint,
		},
		{
			name:     "PR-built Go CD image",
			cdImage:  "ghcr.io/defanglabs/cd:pr-423@sha256:01b3184e0d022c3e1c321015e933d91b4d0ddd552e0ab0826c5a9dd57aebe848",
			expected: goCDEntrypoint,
		},
		{
			name:     "untagged image",
			cdImage:  "public.ecr.aws/defang-io/cd",
			expected: goCDEntrypoint,
		},
		{
			name:     "Fabric-pinned digest-only Go image",
			cdImage:  "ghcr.io/defanglabs/cd@sha256:a550d24f5bafc7b9ddc0149e54419e3eecb15fc9341b1591b6b2a55455fe8136",
			expected: goCDEntrypoint,
		},
		{
			name:     "registry with port is not mistaken for a tag",
			cdImage:  "localhost:5000/cd:public-cd-image-3b4eb980-arm64",
			expected: nodeCDEntrypoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cdEntrypoint(tt.cdImage); !slices.Equal(got, tt.expected) {
				t.Errorf("cdEntrypoint(%q) = %v, expected %v", tt.cdImage, got, tt.expected)
			}
		})
	}
}
