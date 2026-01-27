package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/dryrun"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type ErrInvalidConfigName struct {
	Name string
}

func (e ErrInvalidConfigName) Error() string {
	return fmt.Sprintf("invalid config name; must be alphanumeric or _, cannot start with a number: %q", e.Name)
}

type ConfigSetOptions struct {
	IfNotSet bool
}

type ConfigManager interface {
	ListConfig(ctx context.Context, req *defangv1.ListConfigsRequest) (*defangv1.Secrets, error)
	PutConfig(ctx context.Context, req *defangv1.PutConfigRequest) error
}

func ConfigSet(ctx context.Context, projectName string, provider ConfigManager, name string, value string, options ConfigSetOptions) (bool, error) {
	term.Debugf("Setting config %q in project %q", name, projectName)

	if !pkg.IsValidSecretName(name) {
		return false, ErrInvalidConfigName{Name: name}
	}

	if dryrun.DoDryRun {
		return false, dryrun.ErrDryRun
	}

	if options.IfNotSet {
		// Check if the config is already set
		listResp, err := provider.ListConfig(ctx, &defangv1.ListConfigsRequest{Project: projectName})
		if err != nil {
			return false, fmt.Errorf("failed to get existing config %q: %w", name, err)
		}
		for _, existingName := range listResp.Names {
			if existingName == name {
				return false, nil
			}
		}
	}

	err := provider.PutConfig(ctx, &defangv1.PutConfigRequest{Project: projectName, Name: name, Value: value})
	if err != nil {
		return false, err
	}
	return true, nil
}
