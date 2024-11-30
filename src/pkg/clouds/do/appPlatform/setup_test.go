package appPlatform

import (
	"context"
	"testing"

	"github.com/digitalocean/godo"
)

func TestShellQuote(t *testing.T) {
	// Given
	tests := []struct {
		input    []string
		expected string
	}{
		{
			input:    []string{"true"},
			expected: `"true"`,
		},
		{
			input:    []string{"echo", "hello world"},
			expected: `"echo" "hello world"`,
		},
		{
			input:    []string{"echo", "hello", "world"},
			expected: `"echo" "hello" "world"`,
		},
		{
			input:    []string{"echo", `hello"world`},
			expected: `"echo" "hello\"world"`,
		},
	}

	for _, test := range tests {
		actual := shellQuote(test.input...)
		if actual != test.expected {
			t.Errorf("Expected `%s` but got: `%s`", test.expected, actual)
		}
	}
}

func Test_getImageSourceSpec(t *testing.T) {
	const fakeSha = "sha256:0000000011111111222222223333333344444444555555556666666677777777"
	// Given
	tests := []struct {
		imageURI    string
		overrideTag string
		expected    *godo.ImageSourceSpec
	}{
		{
			imageURI: "docker.io/library/nginx:tagx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				Registry:     "library",
				Repository:   "nginx",
				// Tag:          "tagx",
				Digest: fakeSha + "tagx",
			},
		},
		{
			imageURI: "docker.io/library/nginx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				Registry:     "library",
				Repository:   "nginx",
				// Tag:          "latest",
				Digest: fakeSha + "latest",
			},
		},
		{
			imageURI: "nginx:latest",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				// Tag:        "latest",
				Digest: fakeSha + "latest",
			},
		},
		{
			imageURI: "nginx",
			expected: &godo.ImageSourceSpec{
				RegistryType: "DOCKER_HUB",
				// Registry:     "library",
				Repository: "nginx",
				// Tag:        "latest",
				Digest: fakeSha + "latest",
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
	getDockerhubImageTagDigest = func(ctx context.Context, reg, repo, tag string) (string, error) {
		return fakeSha + tag, nil
	}

	for _, test := range tests {
		t.Run(test.imageURI, func(t *testing.T) {
			t.Setenv("DEFANG_CD_IMAGE", test.imageURI)
			actual, err := getImageSourceSpec(context.Background(), test.overrideTag)
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
