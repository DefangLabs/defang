package compose

import (
	"slices"
	"testing"
)

func TestFindAllBaseImages(t *testing.T) {
	baseimageComposePath := "../../../testdata/base-image/compose.yaml"

	// Load the actual base-image compose.yaml file
	loader := NewLoader(WithPath(baseimageComposePath))
	project, err := loader.LoadProject(t.Context())
	if err != nil {
		t.Fatalf("Failed to load base-image compose.yaml: %v", err)
	}

	images, err := FindAllBaseImages(project)
	if err != nil {
		t.Fatalf("Received unexpected error: %v", err)
	}

	expectedImages := []string{"alpine", "alpine:latest", "ubuntu"}
	if !slices.Equal(images, expectedImages) {
		t.Errorf("Expected images %v, got %v", expectedImages, images)
	}
}
