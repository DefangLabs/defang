package cli

import (
	"context"
	"fmt"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
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

func ConfigSet(ctx context.Context, projectName string, provider client.Provider, name string, value string) error {
	term.Debugf("Setting config %q in project %q", name, projectName)

	if !pkg.IsValidSecretName(name) {
		return ErrInvalidConfigName{Name: name}
	}

	if dryrun.DoDryRun {
		return dryrun.ErrDryRun
	}

	return provider.PutConfig(ctx, &defangv1.PutConfigRequest{Project: projectName, Name: name, Value: value})
}
