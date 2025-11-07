package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestConfigSet(t *testing.T) {
	ctx := t.Context()
	provider := MustHaveProjectNamePutConfigProvider{}

	t.Run("expect no error", func(t *testing.T) {
		err := ConfigSet(ctx, false, "test", provider, "test_name", "test_value")
		if err != nil {
			t.Fatalf("ConfigSet() error = %v", err)
		}
	})

	t.Run("expect error on DryRun", func(t *testing.T) {
		dryrun.DoDryRun = true
		t.Cleanup(func() { dryrun.DoDryRun = false })
		err := ConfigSet(ctx, true, "test", provider, "test_name", "test_value")
		if err != dryrun.ErrDryRun {
			t.Fatalf("Expected dryrun.ErrDryRun, got %v", err)
		}
	})
}

type MustHaveProjectNamePutConfigProvider struct {
	client.Provider
}

func (m MustHaveProjectNamePutConfigProvider) PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error {
	if req.Project == "" {
		return errors.New("project name is missing")
	}
	return nil
}
