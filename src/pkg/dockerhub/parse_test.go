package dockerhub

import "testing"

func TestParseImage(t *testing.T) {
	tests := []struct {
		image string
		want  Image
	}{
		{
			image: "docker.io/pulumi/pulumi:latest",
			want: Image{
				Image:    "docker.io/pulumi/pulumi",
				Registry: "docker.io",
				Repo:     "pulumi/pulumi",
				Tag:      "latest",
			},
		},
		{
			image: "alpine:latest",
			want: Image{
				Image: "alpine",
				Repo:  "alpine",
				Tag:   "latest",
			},
		},
		{
			image: "redis",
			want: Image{
				Image: "redis",
				Repo:  "redis",
			},
		},
		{
			image: "defangio/cd@sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			want: Image{
				Image:    "defangio/cd",
				Registry: "",
				Repo:     "defangio/cd",
				Digest:   "sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			},
		},
		{
			image: "docker.io/pulumi/pulumi:latest@sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			want: Image{
				Image:    "docker.io/pulumi/pulumi",
				Registry: "docker.io",
				Repo:     "pulumi/pulumi",
				Tag:      "latest",
				Digest:   "sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			},
		},
		{
			image: "public.ecr.aws/docker/library/alpine:latest",
			want: Image{
				Image:    "public.ecr.aws/docker/library/alpine",
				Registry: "public.ecr.aws",
				Repo:     "docker/library/alpine",
				Tag:      "latest",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got, err := ParseImage(tt.image)
			if err != nil {
				t.Fatalf("ParseImage(%s) got error: %v", tt.image, err)
			}
			if *got != tt.want {
				t.Errorf("ParseImage(%s) got %#v, want %#v", tt.image, got, tt.want)
			}
		})
	}
}
