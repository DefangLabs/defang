package gcp

import (
	"context"
	"testing"
)

func TestProjectIDFromName(t *testing.T) {
}

func TestSetUpCD(t *testing.T) {
	ctx := context.Background()
	b := New(ctx, "testTenantID")
	err := b.setUpCD(ctx)
	if err != nil {
		t.Errorf("setUpCD() error = %v, want nil", err)
	}
}
