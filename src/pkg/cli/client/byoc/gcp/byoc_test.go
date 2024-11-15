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
	account, err := b.AccountInfo(ctx)
	if err != nil {
		t.Errorf("AccountInfo() error = %v, want nil", err)
	}
	t.Logf("account: %+v", account)
	if err := b.setUpCD(ctx); err != nil {
		t.Errorf("setUpCD() error = %v, want nil", err)
	}
}
