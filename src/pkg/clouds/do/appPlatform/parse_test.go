package appPlatform

import "testing"

func TestParseImage(t *testing.T) {
	tests := []struct {
		image string
		want  Image
	}{
		{
			image: "docker.io/pulumi/pulumi:latest",
			want: Image{
				Registry: "docker.io",
				Repo:     "pulumi/pulumi",
				Tag:      "latest",
			},
		},
		{
			image: "redis",
			want: Image{
				Repo: "redis",
			},
		},
		{
			image: "defangio/cd@sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			want: Image{
				Registry: "defangio",
				Repo:     "cd",
				Digest:   "sha256:2e671c45664af2a40cc9e78dfbf3c985c7f89746b8a62712273c158f3436266a",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			got, err := ParseImage(tt.image)
			if err != nil {
				t.Fatalf("ParseImage(%s) got error: %v", tt.image, err)
			}
			if got.Registry != tt.want.Registry || got.Repo != tt.want.Repo || got.Tag != tt.want.Tag || got.Digest != tt.want.Digest {
				t.Errorf("ParseImage(%s) got %v, want %v", tt.image, got, tt.want)
			}
		})
	}
}
