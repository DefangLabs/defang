package mcp

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSetupKnowledgeBase covers happy path and HTTP error handling.
func TestSetupKnowledgeBase(t *testing.T) {
	// Spin up a mock server to serve the two JSON files.
	mux := http.NewServeMux()
	expected := map[string]string{
		"knowledge_base.json":   `{"sections":[{"title":"Intro","content":"KB"}]}`,
		"samples_examples.json": `{"samples":[{"name":"sample","desc":"Example"}]}`,
	}
	mux.HandleFunc("/data/knowledge_base.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, expected["knowledge_base.json"])
	})
	mux.HandleFunc("/data/samples_examples.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, expected["samples_examples.json"])
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Override globals
	oldBase := AskDefangBaseURL
	AskDefangBaseURL = srv.URL
	defer func() { AskDefangBaseURL = oldBase }()

	prevStateDir := KnowledgeBaseDir
	tmp := t.TempDir()
	KnowledgeBaseDir = tmp
	defer func() { KnowledgeBaseDir = prevStateDir }()

	if err := SetupKnowledgeBase(); err != nil {
		t.Fatalf("SetupKnowledgeBase() unexpected error: %v", err)
	}

	// Assert files created with exact expected content.
	for _, f := range []string{"knowledge_base.json", "samples_examples.json"} {
		path := filepath.Join(tmp, f)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected file %s to exist: %v", f, err)
		}
		if got, want := string(b), expected[f]; got != want {
			t.Fatalf("unexpected content for %s\nwant: %s\n got: %s", f, want, got)
		}
	}
}

func TestSetupKnowledgeBase_HTTPError(t *testing.T) {
	// Server returns 500 for first file to trigger error path.
	mux := http.NewServeMux()
	mux.HandleFunc("/data/knowledge_base.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kb_error", http.StatusInternalServerError)
	})
	mux.HandleFunc("/data/samples_examples.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	oldBase := AskDefangBaseURL
	AskDefangBaseURL = srv.URL
	defer func() { AskDefangBaseURL = oldBase }()

	prevStateDir := KnowledgeBaseDir
	tmp := t.TempDir()
	KnowledgeBaseDir = tmp
	defer func() { KnowledgeBaseDir = prevStateDir }()

	if err := SetupKnowledgeBase(); err == nil {
		t.Fatalf("expected error when first file returns 500")
	}

	// Only first file should have been created (empty) and second not attempted due to early return.
	if _, err := os.Stat(filepath.Join(tmp, "samples_examples.json")); err == nil {
		t.Fatalf("did not expect samples_examples.json to be created after failure on first file")
	}

	// Verify failing file exists but is empty (we created then errored before write).
	_, err := os.Stat(filepath.Join(tmp, "knowledge_base.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected failing file not to exist: %v", err)
	}
}
