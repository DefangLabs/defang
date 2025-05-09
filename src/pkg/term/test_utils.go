package term

import (
	"bytes"
	"os"
	"testing"
)

func SetupTestTerm(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	testTerm := NewTerm(os.Stdin, &stdout, &stderr)
	testTerm.ForceColor(true)
	oldTerm := DefaultTerm
	DefaultTerm = testTerm
	t.Cleanup(func() {
		DefaultTerm = oldTerm
	})

	return &stdout, &stderr
}
