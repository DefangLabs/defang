package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestConfigGet(t *testing.T) {
	ctx := t.Context()
	provider := MustHaveProjectNameGetConfigProvider{}

	t.Run("no config", func(t *testing.T) {
		_, err := ConfigGet(ctx, "project", []string{"no_config"}, provider)
		if err != nil {
			t.Fatalf("ConfigGet() error = %v", err)
		}
	})

	t.Run("expect error on DryRun", func(t *testing.T) {
		dryrun.DoDryRun = true
		t.Cleanup(func() { dryrun.DoDryRun = false })
		_, err := ConfigGet(ctx, "project", []string{"has_config"}, provider)
		if err != dryrun.ErrDryRun {
			t.Fatalf("Expected dryrun.ErrDryRun, got %v", err)
		}
	})

	t.Run("normal case", func(t *testing.T) {
		resp, err := ConfigGet(ctx, "project", []string{"has_config"}, provider)
		if err != nil {
			t.Fatalf("ConfigGet() error = %v", err)
		}
		if len(resp.Configs) != 1 {
			t.Fatalf("Expected 1 config, got %d", len(resp.Configs))
		}
		if resp.Configs[0].Name != "has_config" || resp.Configs[0].Value != "some_value" {
			t.Fatalf("Unexpected config returned: %+v", resp.Configs[0])
		}
	})
}

type MustHaveProjectNameGetConfigProvider struct {
	client.Provider
}

func (m MustHaveProjectNameGetConfigProvider) GetConfigs(ctx context.Context, req *defangv1.GetConfigsRequest) (*defangv1.GetConfigsResponse, error) {
	resp := &defangv1.GetConfigsResponse{
		Configs: []*defangv1.Config{
			{
				Project: "project",
				Name:    "has_config",
				Value:   "some_value",
			},
		},
	}

	if req.Configs[0].Project == "" {
		return nil, errors.New("project name is missing")
	}

	if len(req.Configs[0].Name) == 0 {
		return nil, errors.New("config name is missing")
	}

	return resp, nil
}
