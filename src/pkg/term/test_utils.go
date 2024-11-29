package term

import (
	"bytes"
	"io"
	"testing"
)

func SetupTestTerm(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	testTerm := NewTerm(&stdout, io.MultiWriter(&stderr))
	testTerm.ForceColor(true)
	defaultTerm := DefaultTerm
	DefaultTerm = testTerm
	t.Cleanup(func() {
		DefaultTerm = defaultTerm
	})

	return &stdout, &stderr
}
