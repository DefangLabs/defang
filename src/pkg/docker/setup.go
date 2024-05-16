package docker

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/docker/docker/api/types"

	pkgtypes "github.com/DefangLabs/defang/src/pkg/types"
)

func (d *Docker) SetUp(ctx context.Context, containers []pkgtypes.Container) error {
	if len(containers) != 1 {
		return errors.New("only one container is supported with docker driver")
	}
	task := containers[0]
	rc, err := d.ImagePull(ctx, task.Image, types.ImagePullOptions{Platform: task.Platform})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(contextAwareWriter{ctx, os.Stderr}, rc) // FIXME: this outputs JSON to stderr
	d.image = task.Image
	d.memory = task.Memory
	d.platform = task.Platform
	return err
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
