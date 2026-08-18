package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

type mockCleaner struct {
	orphans     []client.OrphanResource
	discoverErr error
	cleanupErr  map[string]error // keyed by OrphanResource.ID
	cleaned     []string
}

func (m *mockCleaner) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	return m.orphans, m.discoverErr
}

func (m *mockCleaner) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	if err := m.cleanupErr[r.ID]; err != nil {
		return err
	}
	m.cleaned = append(m.cleaned, r.ID)
	return nil
}

func orphan(id, category string) client.OrphanResource {
	return client.OrphanResource{ID: id, Category: category, Name: id, Action: "do the thing"}
}

func TestCleanupNoOrphans(t *testing.T) {
	m := &mockCleaner{}
	summary, err := Cleanup(t.Context(), m, "myproj", CleanupOptions{})
	if err != nil {
		t.Fatalf("Cleanup() failed: %v", err)
	}
	if summary != `No leftover resources found for project "myproj" that are blocking cleanup.` {
		t.Errorf("unexpected summary: %s", summary)
	}
}

func TestCleanupDiscoverError(t *testing.T) {
	m := &mockCleaner{discoverErr: errors.New("boom")}
	if _, err := Cleanup(t.Context(), m, "myproj", CleanupOptions{}); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestCleanupReportOnly(t *testing.T) {
	m := &mockCleaner{orphans: []client.OrphanResource{orphan("alb:1", "alb"), orphan("ecr:repo", "ecr")}}
	summary, err := Cleanup(t.Context(), m, "myproj", CleanupOptions{ReportOnly: true})
	if err != nil {
		t.Fatalf("Cleanup() failed: %v", err)
	}
	if len(m.cleaned) != 0 {
		t.Errorf("ReportOnly must not clean anything up, got: %v", m.cleaned)
	}
	if summary == "" {
		t.Error("expected a non-empty summary directing the user to re-run interactively or with --yes")
	}
}

func TestCleanupAutoConfirm(t *testing.T) {
	m := &mockCleaner{
		orphans: []client.OrphanResource{orphan("alb:1", "alb"), orphan("ecr:repo", "ecr")},
		cleanupErr: map[string]error{
			"ecr:repo": errors.New("access denied"),
		},
	}
	// AutoConfirm must win even if ReportOnly is also set (mirrors the CLI: --yes overrides
	// the non-interactive report-only fallback).
	summary, err := Cleanup(t.Context(), m, "myproj", CleanupOptions{AutoConfirm: true, ReportOnly: true})
	if err != nil {
		t.Fatalf("Cleanup() failed: %v", err)
	}
	if len(m.cleaned) != 1 || m.cleaned[0] != "alb:1" {
		t.Errorf("expected only alb:1 to be cleaned, got: %v", m.cleaned)
	}
	want := "1 cleaned, 0 skipped, 1 failed. Run 'defang down' so Pulumi can finish removing the now-unblocked resources."
	if summary != want {
		t.Errorf("unexpected summary:\n got:  %s\n want: %s", summary, want)
	}
}
