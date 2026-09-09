package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/elicitations"
)

// mockCleaner is a Provider that also implements the optional OrphanCleaner capability, recording
// the order in which resources were cleaned so the ordering guarantee can be asserted.
type mockCleaner struct {
	client.MockProvider
	orphans     []client.OrphanResource
	discoverErr error
	failIDs     map[string]error
	cleaned     []string
}

func (m *mockCleaner) DiscoverOrphans(ctx context.Context, projectName string) ([]client.OrphanResource, error) {
	return m.orphans, m.discoverErr
}

func (m *mockCleaner) CleanupOrphan(ctx context.Context, r client.OrphanResource) error {
	if err, ok := m.failIDs[r.ID]; ok {
		return err
	}
	m.cleaned = append(m.cleaned, r.ID)
	return nil
}

// stubController is an elicitations.Controller with a canned set of answers.
type stubController struct {
	elicitations.Controller
	supported bool
	answers   []string
	asked     []string
	err       error
}

func (s *stubController) IsSupported() bool { return s.supported }

func (s *stubController) RequestEnum(ctx context.Context, message, field string, options []string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	s.asked = append(s.asked, message)
	if len(s.answers) == 0 {
		return "no", nil
	}
	answer := s.answers[0]
	s.answers = s.answers[1:]
	return answer, nil
}

func orphans(ids ...string) []client.OrphanResource {
	var rs []client.OrphanResource
	for _, id := range ids {
		rs = append(rs, client.OrphanResource{ID: id, Category: "test", Name: id, Action: "remove " + id})
	}
	return rs
}

func TestCleanupUnsupportedProvider(t *testing.T) {
	_, err := Cleanup(context.Background(), client.MockProvider{}, &stubController{}, "proj", CleanupOptions{})
	if !errors.Is(err, ErrCleanupUnsupported) {
		t.Errorf("expected ErrCleanupUnsupported, got %v", err)
	}
}

func TestCleanupNothingFound(t *testing.T) {
	result, err := Cleanup(context.Background(), &mockCleaner{}, &stubController{supported: true}, "proj", CleanupOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Found != 0 || result.Report != "" {
		t.Errorf("expected an empty result, got %+v", result)
	}
}

func TestCleanupDiscoverError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := Cleanup(context.Background(), &mockCleaner{discoverErr: sentinel}, &stubController{supported: true}, "proj", CleanupOptions{})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected the discovery error to be wrapped, got %v", err)
	}
}

// A dry run must never call CleanupOrphan, and must not ask for confirmation either.
func TestCleanupDryRun(t *testing.T) {
	cleaner := &mockCleaner{orphans: orphans("a", "b")}
	ec := &stubController{supported: true, answers: []string{"yes", "yes"}}
	result, err := Cleanup(context.Background(), cleaner, ec, "proj", CleanupOptions{DryRun: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ReportOnly {
		t.Error("expected ReportOnly for a dry run")
	}
	if len(cleaner.cleaned) != 0 {
		t.Errorf("dry run cleaned %v", cleaner.cleaned)
	}
	if len(ec.asked) != 0 {
		t.Errorf("dry run asked for confirmation: %v", ec.asked)
	}
	if !strings.Contains(result.Report, "would remove a") {
		t.Errorf("report should describe the pending action, got %q", result.Report)
	}
}

// Without elicitation support there is no one to confirm, so the run must degrade to a report
// rather than deleting resources unasked.
func TestCleanupNonInteractiveReportsOnly(t *testing.T) {
	cleaner := &mockCleaner{orphans: orphans("a")}
	result, err := Cleanup(context.Background(), cleaner, &stubController{supported: false}, "proj", CleanupOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ReportOnly {
		t.Error("expected ReportOnly when elicitation is unsupported")
	}
	if len(cleaner.cleaned) != 0 {
		t.Errorf("cleaned without confirmation: %v", cleaner.cleaned)
	}
}

// --yes must work even where elicitation is unavailable, which is the CI case.
func TestCleanupYesWithoutElicitation(t *testing.T) {
	cleaner := &mockCleaner{orphans: orphans("a", "b")}
	ec := &stubController{supported: false}
	result, err := Cleanup(context.Background(), cleaner, ec, "proj", CleanupOptions{Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ReportOnly {
		t.Error("--yes should not degrade to a report")
	}
	if result.Cleaned != 2 {
		t.Errorf("expected 2 cleaned, got %d", result.Cleaned)
	}
	if len(ec.asked) != 0 {
		t.Errorf("--yes asked for confirmation: %v", ec.asked)
	}
}

// Providers return orphans in dependency order (GCP must delete instance templates before the
// subnet, and the network last), so Cleanup must preserve that order exactly.
func TestCleanupPreservesOrder(t *testing.T) {
	ids := []string{"template", "peering", "range", "subnet", "network"}
	cleaner := &mockCleaner{orphans: orphans(ids...)}
	_, err := Cleanup(context.Background(), cleaner, &stubController{supported: false}, "proj", CleanupOptions{Yes: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(cleaner.cleaned, ",") != strings.Join(ids, ",") {
		t.Errorf("order not preserved: got %v want %v", cleaner.cleaned, ids)
	}
}

func TestCleanupSkipAndFailureCounts(t *testing.T) {
	cleaner := &mockCleaner{
		orphans: orphans("a", "b", "c"),
		failIDs: map[string]error{"c": errors.New("still in use")},
	}
	ec := &stubController{supported: true, answers: []string{"yes", "no", "yes"}}
	result, err := Cleanup(context.Background(), cleaner, ec, "proj", CleanupOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Cleaned != 1 || result.Skipped != 1 || result.Failed != 1 {
		t.Errorf("got cleaned=%d skipped=%d failed=%d, want 1/1/1", result.Cleaned, result.Skipped, result.Failed)
	}
	if len(cleaner.cleaned) != 1 || cleaner.cleaned[0] != "a" {
		t.Errorf("expected only \"a\" cleaned, got %v", cleaner.cleaned)
	}
	for _, want := range []string{"— done", "— skipped", "— failed: still in use"} {
		if !strings.Contains(result.Report, want) {
			t.Errorf("report missing %q:\n%s", want, result.Report)
		}
	}
}

// A confirmation transport that breaks must abort rather than silently skip the rest, but the
// report gathered so far is still returned so the caller can show what already happened.
func TestCleanupConfirmationError(t *testing.T) {
	sentinel := errors.New("transport closed")
	cleaner := &mockCleaner{orphans: orphans("a")}
	result, err := Cleanup(context.Background(), cleaner, &stubController{supported: true, err: sentinel}, "proj", CleanupOptions{})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected the confirmation error to be wrapped, got %v", err)
	}
	if len(cleaner.cleaned) != 0 {
		t.Errorf("cleaned despite a failed confirmation: %v", cleaner.cleaned)
	}
	if result.Report == "" {
		t.Error("expected the partial report to be returned")
	}
}
