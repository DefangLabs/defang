package docker

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/DefangLabs/defang/src/pkg/clouds"
	"github.com/docker/docker/api/types/image"
)

func (d *Docker) SetUp(ctx context.Context, containers []clouds.Container) (bool, error) {
	if len(containers) != 1 {
		return false, errors.New("only one container is supported with docker driver")
	}
	task := containers[0]
	rc, err := d.ImagePull(ctx, task.Image, image.PullOptions{Platform: task.Platform})
	if err != nil {
		return false, err
	}
	defer rc.Close()
	_, err = io.Copy(contextAwareWriter{ctx, os.Stderr}, rc) // FIXME: this outputs JSON to stderr
	d.image = task.Image
	d.memory = task.Memory
	d.platform = task.Platform
	return false, err
}

type contextAwareWriter struct {
	ctx context.Context
	io.Writer
}

func (cw contextAwareWriter) Write(p []byte) (n int, err error) {
	select {
	case <-cw.ctx.Done(): // Detect context cancelation
		return 0, cw.ctx.Err()
	default:
		return cw.Writer.Write(p)
	}
}
