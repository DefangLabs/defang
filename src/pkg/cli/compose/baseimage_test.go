package compose

import (
	"testing"
)

func TestFindAllBaseImages(t *testing.T) {
	t.Run("Reproduce railpack issue with real compose.yaml", func(t *testing.T) {

		baseimageComposePath := "../../../testdata/base-image/compose.yaml"

		// Load the actual base-image compose.yaml file
		loader := NewLoader(WithPath(baseimageComposePath))
		project, err := loader.LoadProject(t.Context())
		if err != nil {
			t.Fatalf("Failed to load base-image compose.yaml: %v", err)
		}

		_, err = FindAllBaseImages(project)

		// Verify we get the expected error
		if err == nil {
			t.Log("Expected error when Dockerfile doesn't exist for railpack services, but got nil")
		} else {
			t.Fatalf("Received expected error: %v", err)
		}
	})
}
