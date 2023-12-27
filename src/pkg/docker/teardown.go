package docker

import (
	"context"
	"errors"
)

func (d Docker) TearDown(ctx context.Context) error {
	return errors.New("not implemented")
}
