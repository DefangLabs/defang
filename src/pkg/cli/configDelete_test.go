package cli

import (
	"context"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestConfigDelete(t *testing.T) {
	ctx := context.Background()
	provider := MockConfigDeleteProvider{}

	t.Run("expect no error", func(t *testing.T) {
		if err := ConfigDelete(ctx, "test", provider, "test_name"); err != nil {
			t.Fatalf("ConfigDelete() error = %v", err)
		}
	})

	t.Run("expect error on DryRun", func(t *testing.T) {
		DoDryRun = true
		defer func() { DoDryRun = false }()

		if err := ConfigDelete(ctx, "test", provider, "test_name"); err != ErrDryRun {
			t.Fatalf("Expected ErrDryRun, got %v", err)
		}
	})
}

type MockConfigDeleteProvider struct {
	client.Provider
}

func (MockConfigDeleteProvider) DeleteConfig(ctx context.Context, req *defangv1.Secrets) error {
	return nil
}
