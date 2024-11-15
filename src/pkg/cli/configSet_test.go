package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/compose"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestConfigSet(t *testing.T) {
	ctx := context.Background()
	loader := client.MockLoader{Project: &compose.Project{Name: "test"}}
	provider := MustHaveProjectNamePutConfigProvider{}
	err := ConfigSet(ctx, loader, provider, "test_name", "test_value")
	if err != nil {
		t.Errorf("ConfigSet() error = %v", err)
		return
	}
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
