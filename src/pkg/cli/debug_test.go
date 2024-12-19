package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestFindMathingProjectFiles(t *testing.T) {
	project, err := compose.NewLoader(compose.WithPath("../../testdata/debugproj/compose.yaml")).LoadProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Test that the correct files are found for debugging: compose.yaml + only files from the failing service
	files := findMatchingProjectFiles(project, []string{"failing", "failing-image"})
	expected := []*defangv1.File{
		{Name: "compose.yaml", Content: "services:\n  failing:\n    build: ./app\n  ok:\n    build: .\n"},
		{Name: "app/Dockerfile", Content: "FROM scratch"},
		{Name: "app/main.js", Content: "// This file should be sent to the debugger"},
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

type MustHaveProjectNameQueryProvider struct {
	client.Provider
}

func (m MustHaveProjectNameQueryProvider) Query(ctx context.Context, req *defangv1.DebugRequest) error {
	if req.Project == "" {
		return errors.New("project name is missing")
	}
	return nil
}

type MockDebugFabricClient struct {
	client.FabricClient
}

func (m MockDebugFabricClient) Debug(ctx context.Context, req *defangv1.DebugRequest) (*defangv1.DebugResponse, error) {
	return &defangv1.DebugResponse{}, nil
}

func TestQueryHasProject(t *testing.T) {
	project, err := compose.NewLoader(compose.WithPath("../../testdata/debugproj/compose.yaml")).LoadProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	provider := MustHaveProjectNameQueryProvider{}
	fabricClient := MockDebugFabricClient{}

	if err := Debug(context.Background(), fabricClient, provider, "etag", project, []string{"service"}); err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	project.Name = ""

	if err := Debug(context.Background(), fabricClient, provider, "etag", project, []string{"service"}); err == nil {
		t.Error("expected error, got nil")
	} else {
		if err.Error() != "project name is missing" {
			t.Errorf("expected error %q, got %q", "project name is missing", err.Error())
		}
	}
}
