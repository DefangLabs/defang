package docker

import (
	"context"
)

func (d *Docker) SetUp(ctx context.Context, image string) error {
	d.image = image
	return nil
}
