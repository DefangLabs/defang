package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestDebug(t *testing.T) {
	project, err := compose.NewLoaderWithPath("../../tests/debugproj/compose.yaml").LoadProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Test that the correct files are found for debugging: compose.yaml + only files from the failing service
	files := findMatchingProjectFiles(project, []string{"failing", "failing-image"})
	expected := []*defangv1.File{
		{Name: "compose.yaml", Content: "services:\n  failing:\n    build: ./app\n  ok:\n    build: .\n"},
		{Name: "Dockerfile", Content: "FROM scratch"},
		{Name: "main.js", Content: "// This file should be sent to the debugger"},
	}
	if len(files) != len(expected) {
		t.Fatalf("expected %d files, got %d", len(expected), len(files))
	}
	for i, file := range files {
		if file.Name != expected[i].Name {
			t.Errorf("expected file name %q, got: %q", expected[i].Name, file.Name)
		}
		if file.Content != expected[i].Content {
			t.Errorf("expected file content %q, got: %q", expected[i].Content, file.Content)
		}
	}
}
