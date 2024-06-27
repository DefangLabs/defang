package cli

import (
	"context"
	"testing"
)

func TestInitFromSamples(t *testing.T) {
	err := InitFromSamples(context.Background(), t.TempDir(), []string{"nonexisting"})
	if err == nil {
		t.Fatal("Expected test to fail")
	}
	if err.Error() != "sample not found" {
		t.Error("Expected 'sample not found' error")
	}
}
