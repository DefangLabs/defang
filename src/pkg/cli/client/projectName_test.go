package client

import (
	"context"
	"errors"
	"testing"

	composeTypes "github.com/compose-spec/compose-go/v2/types"
)

func TestLoadProjectNameWithFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("with project", func(t *testing.T) {
		loader := MockLoader{Project: composeTypes.Project{Name: "test-project"}}
		projectName, err := LoadProjectNameWithFallback(ctx, loader, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if projectName != "test-project" {
			t.Fatalf("expected project name 'test-project', got %q", projectName)
		}
	})

	t.Run("no local project, no fallback", func(t *testing.T) {
		loader := MockLoader{Error: errors.New("no local project")}
		provider := mockRemoteProjectName{
			Error: errors.New("no remote project"),
		}
		projectName, err := LoadProjectNameWithFallback(ctx, loader, provider)
		if err == nil {
			t.Fatalf("expected error, got project name %q", projectName)
		}
		if expected, got := "no local project and no remote project", err.Error(); expected != got {
			t.Fatalf("expected error message %q, got %q", expected, got)
		}
	})

	t.Run("no local project, with fallback", func(t *testing.T) {
		loader := MockLoader{Error: errors.New("no local project")}
		provider := mockRemoteProjectName{
			ProjectName: "test-project",
		}
		projectName, err := LoadProjectNameWithFallback(ctx, loader, provider)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if projectName != "test-project" {
			t.Fatalf("expected project name 'test-project', got %q", projectName)
		}
	})
}

type mockRemoteProjectName struct {
	Provider
	ProjectName string
	Error       error
}

func (m mockRemoteProjectName) RemoteProjectName(context.Context) (string, error) {
	return m.ProjectName, m.Error
}
