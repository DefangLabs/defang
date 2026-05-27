package scaleway

import (
	"testing"
)

func TestS3Endpoint(t *testing.T) {
	tests := []struct {
		region string
		want   string
	}{
		{"fr-par", "https://s3.fr-par.scw.cloud"},
		{"nl-ams", "https://s3.nl-ams.scw.cloud"},
		{"pl-waw", "https://s3.pl-waw.scw.cloud"},
	}
	for _, tt := range tests {
		t.Run(tt.region, func(t *testing.T) {
			if got := S3Endpoint(tt.region); got != tt.want {
				t.Errorf("S3Endpoint(%q) = %q, want %q", tt.region, got, tt.want)
			}
		})
	}
}

func TestNewS3Client(t *testing.T) {
	client := NewS3Client("fr-par", "SCWAK", "secret")
	if client == nil {
		t.Fatal("NewS3Client returned nil")
	}
}

func TestGetRegistryEndpoint(t *testing.T) {
	tests := []struct {
		region    string
		namespace string
		want      string
	}{
		{"fr-par", "my-ns", "rg.fr-par.scw.cloud/my-ns"},
		{"nl-ams", "defang-cd", "rg.nl-ams.scw.cloud/defang-cd"},
	}
	for _, tt := range tests {
		t.Run(tt.region+"/"+tt.namespace, func(t *testing.T) {
			if got := GetRegistryEndpoint(tt.region, tt.namespace); got != tt.want {
				t.Errorf("GetRegistryEndpoint(%q, %q) = %q, want %q", tt.region, tt.namespace, got, tt.want)
			}
		})
	}
}
