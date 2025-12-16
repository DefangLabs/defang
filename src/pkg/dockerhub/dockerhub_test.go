package dockerhub

import "testing"

func TestIsDockerHubImage(t *testing.T) {
	tests := []struct {
		image string
		want  bool
	}{
		{"docker.io/pulumi/pulumi:latest", true},
		{"index.docker.io/library/redis:latest", true},
		{"redis", true},
		{"defangio/cd@sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a", true},
		{"gcr.io/google-containers/pause:3.1", false},
		{"quay.io/coreos/etcd:v3.3.10", false},
		{"public.ecr.aws/docker/library/alpine:latest", false},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got := IsDockerHubImage(tt.image)
			if got != tt.want {
				t.Errorf("IsDockerHubImage(%s) got %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}
