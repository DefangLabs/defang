package gcp

import (
	"encoding/base64"
	"testing"
)

func TestSetUpCD(t *testing.T) {
	t.Skip("skipping test")
	ctx := t.Context()
	b := NewByocProvider(ctx, "testTenantID")
	account, err := b.AccountInfo(ctx)
	if err != nil {
		t.Errorf("AccountInfo() error = %v, want nil", err)
	}
	t.Logf("account: %+v", account)
	if err := b.setUpCD(ctx); err != nil {
		t.Errorf("setUpCD() error = %v, want nil", err)
	}

	payload := base64.StdEncoding.EncodeToString([]byte(`services:
  nginx:
    image: nginx:1-alpine
    ports:
      - "8080:80"
`))
	cmd := cdCommand{
		Project: "testproj",
		Command: []string{"up", payload},
	}

	if op, err := b.runCdCommand(ctx, cmd); err != nil {
		t.Errorf("BootstrapCommand() error = %v, want nil", err)
	} else {
		t.Logf("BootstrapCommand() = %v", op)
	}
}
