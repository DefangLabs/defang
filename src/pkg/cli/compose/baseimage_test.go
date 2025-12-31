package compose

import (
	"testing"
)

func TestFindAllBaseImages(t *testing.T) {
	t.Run("TestFindAllBaseImages", func(t *testing.T) {

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
		if len(images) != len(expectedImages) {
			t.Fatalf("Expected %d images, got %d", len(expectedImages), len(images))
		}

		for i, img := range expectedImages {
			if images[i] != img {
				t.Errorf("Expected image %s, got %s", img, images[i])
			}
		}
	})
}
