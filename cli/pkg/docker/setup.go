package docker

import (
	"context"

	"github.com/docker/docker/api/types"
)

func (d *Docker) SetUp(ctx context.Context, image string, memory uint64) error {
	rc, err := d.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer rc.Close()
	// _, err = io.Copy(os.Stdout, rc) TODO: do we need this?
	d.image = image
	d.memory = memory
	return err
}
