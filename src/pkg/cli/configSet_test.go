package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

func TestConfigSet(t *testing.T) {
	ctx := context.Background()
	provider := MustHaveProjectNamePutConfigProvider{}
	err := ConfigSet(ctx, "test", provider, "test_name", "test_value")
	if err != nil {
		t.Fatalf("ConfigSet() error = %v", err)
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
