package byoc

import "testing"

func TestIsDefangMvpCDImage(t *testing.T) {
	tests := []struct {
		name    string
		cdImage string
		mvp     bool
	}{
		{"defang-mvp AWS public image", "public.ecr.aws/defang-io/cd:public-cd-image-3b4eb980-arm64@sha256:5233e7e84a8a128ba47f36db3ce0eff10ae6242adb5568c7b1e2e0474b38a677", true},
		{"defang-mvp AWS nodejs image", "381492210770.dkr.ecr.us-west-2.amazonaws.com/defang-cd-ecr-public/defang-io/cd:nodejs-cd-image-707883e0-x86_64", true},
		{"defang-mvp GCP image", "us-docker.pkg.dev/defang-io/cd/cd:public-gcp-6d216eec-x86_64", true},
		{"defang-mvp fallback tag", "public.ecr.aws/defang-io/cd:public-beta", true},
		{"defang-mvp GCP fallback tag", "us-docker.pkg.dev/defang-io/cd/cd:public-gcp-beta", true},
		{"registry with port is not mistaken for a tag", "localhost:5000/cd:public-cd-image-3b4eb980-arm64", true},
		{"released pulumi-defang CD image", "public.ecr.aws/defang-io/cd:v2.6.0-aws@sha256:5e7c7a1931fda2ce7def36a1da3daefa1d1221b573d41a3131b299e15543f8a7", false},
		{"prerelease pulumi-defang CD image", "ghcr.io/defanglabs/cd:2.6.0-alpha-37499b67", false},
		{"PR-built pulumi-defang CD image", "ghcr.io/defanglabs/cd:pr-423@sha256:01b3184e0d022c3e1c321015e933d91b4d0ddd552e0ab0826c5a9dd57aebe848", false},
		{"Fabric-pinned digest-only pulumi-defang image", "ghcr.io/defanglabs/cd@sha256:a550d24f5bafc7b9ddc0149e54419e3eecb15fc9341b1591b6b2a55455fe8136", false},
		{"untagged image", "public.ecr.aws/defang-io/cd", false},
		{"no image", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDefangMvpCDImage(tt.cdImage); got != tt.mvp {
				t.Errorf("IsDefangMvpCDImage(%q) = %v, want %v", tt.cdImage, got, tt.mvp)
			}
		})
	}
}
