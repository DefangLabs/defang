package acr

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadLogChunk_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		// empty body
	}))
	defer srv.Close()

	var yielded []string
	yield := func(line string, err error) bool {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		yielded = append(yielded, line)
		return true
	}

	newOffset, err := readLogChunk(t.Context(), srv.URL, 0, yield)
	if err != nil {
		t.Fatalf("unexpected sentinel error: %v", err)
	}
	if newOffset != 0 {
		t.Errorf("expected offset 0, got %d", newOffset)
	}
	if len(yielded) != 0 {
		t.Errorf("expected no lines yielded, got %d", len(yielded))
	}
}

func TestReadLogChunk_CompleteLines(t *testing.T) {
	body := "line1\nline2\nline3\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	var yielded []string
	yield := func(line string, err error) bool {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		yielded = append(yielded, line)
		return true
	}

	newOffset, err := readLogChunk(t.Context(), srv.URL, 0, yield)
	if err != nil {
		t.Fatalf("unexpected sentinel error: %v", err)
	}
	// strings.Split on a trailing-newline body produces one extra empty element,
	// each element (including the empty one) adds len(line)+1 to the offset.
	expectedOffset := int64(len(body)) + 1 // trailing empty element adds +1
	if newOffset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, newOffset)
	}
	// 3 real lines + 1 empty string from the trailing newline split
	if len(yielded) != 4 {
		t.Errorf("expected 4 yielded elements, got %d: %v", len(yielded), yielded)
	}
}

func TestReadLogChunk_IncompleteLastLine(t *testing.T) {
	// Last line has no trailing newline — should be held back.
	body := "line1\nline2\nincomplete"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	var yielded []string
	yield := func(line string, err error) bool {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		yielded = append(yielded, line)
		return true
	}

	newOffset, _ := readLogChunk(t.Context(), srv.URL, 0, yield)
	// Only complete lines (line1, line2) should advance the offset.
	expectedOffset := int64(len("line1\nline2\n"))
	if newOffset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, newOffset)
	}
	if len(yielded) != 2 {
		t.Errorf("expected 2 lines, got %d: %v", len(yielded), yielded)
	}
}

func TestReadLogChunk_RangeNotSatisfiable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer srv.Close()

	called := false
	yield := func(line string, err error) bool {
		called = true
		return true
	}

	newOffset, err := readLogChunk(t.Context(), srv.URL, 42, yield)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if newOffset != 42 {
		t.Errorf("expected offset unchanged at 42, got %d", newOffset)
	}
	if called {
		t.Error("yield should not have been called for 416 response")
	}
}

func TestReadLogChunk_UnexpectedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var gotErr error
	yield := func(line string, err error) bool {
		gotErr = err
		return true
	}

	readLogChunk(t.Context(), srv.URL, 0, yield)
	if gotErr == nil {
		t.Error("expected an error for unexpected status code")
	}
}

func TestReadLogChunk_YieldStopsIteration(t *testing.T) {
	body := "line1\nline2\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		fmt.Fprint(w, body)
	}))
	defer srv.Close()

	callCount := 0
	yield := func(line string, err error) bool {
		callCount++
		return false // stop after first line
	}

	_, sentinelErr := readLogChunk(t.Context(), srv.URL, 0, yield)
	if sentinelErr != errStopped {
		t.Errorf("expected errStopped, got %v", sentinelErr)
	}
	if callCount != 1 {
		t.Errorf("expected yield called once, got %d", callCount)
	}
}
