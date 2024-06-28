package cli

import (
	"context"
	"errors"
	"testing"
)

func TestInitFromSamples(t *testing.T) {
	err := InitFromSamples(context.Background(), t.TempDir(), []string{"nonexisting"})
	if err == nil {
		t.Fatal("Expected test to fail")
	}
	if !errors.Is(err, ErrSampleNotFound) {
		t.Errorf("Expected error to be %v, got %v", ErrSampleNotFound, err)
	}
}
