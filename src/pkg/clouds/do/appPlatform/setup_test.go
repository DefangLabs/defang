package appPlatform

import (
	"testing"

	"github.com/digitalocean/godo"
)

func Test_getImageSourceSpec(t *testing.T) {
	// Given
	tests := []struct {
		imageURI string
		expected *godo.ImageSourceSpec
	}{
		{
			imageURI: "docker.io/library/nginx:tagx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				Registry:     "library",
				Repository:   "nginx",
				Tag:          "tagx",
			},
		},
		{
			imageURI: "docker.io/library/nginx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				Registry:     "library",
				Repository:   "nginx",
				Tag:          "latest",
			},
		},
		{
			imageURI: "nginx:latest",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				Tag:        "latest",
			},
		},
		{
			imageURI: "nginx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				Tag:        "latest",
			},
		},
		{
			imageURI: "nginx@sha256:01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				Digest:     "sha256:01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b",
			},
		},
		{
			imageURI: "nginx:latest@sha256:01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				Digest:     "sha256:01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.imageURI, func(t *testing.T) {
			actual, err := getImageSourceSpec(test.imageURI)
			if err != nil {
				t.Fatalf("Expected no error but got: %s", err)
			}
			if expected, got := test.expected.RegistryType, actual.RegistryType; expected != got {
				t.Errorf("Expected RegistryType `%s` but got: `%s`", expected, got)
			}
			if expected, got := test.expected.Registry, actual.Registry; expected != got {
				t.Errorf("Expected Registry `%s` but got: `%s`", expected, got)
			}
			if expected, got := test.expected.Digest, actual.Digest; expected != got {
				t.Errorf("Expected Digest `%s` but got: `%s`", expected, got)
			}
			if expected, got := test.expected.Repository, actual.Repository; expected != got {
				t.Errorf("Expected Repository `%s` but got: `%s`", expected, got)
			}
			if expected, got := test.expected.Tag, actual.Tag; expected != got {
				t.Errorf("Expected Tag `%s` but got: `%s`", expected, got)
			}
		})
	}
}
