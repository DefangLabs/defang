package docker

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/cli/client"
)

func (d Docker) TearDown(ctx context.Context) error {
	return client.ErrNotImplemented("not implemented")
}
