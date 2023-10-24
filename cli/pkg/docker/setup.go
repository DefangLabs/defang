package docker

import (
	"context"
	"io"
	"os"

	"github.com/docker/docker/api/types"
)

func (d *Docker) SetUp(ctx context.Context, image string, memory uint64, platform string) error {
	rc, err := d.ImagePull(ctx, image, types.ImagePullOptions{Platform: platform})
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = io.Copy(contextAwareWriter{ctx, os.Stderr}, rc)
	d.image = image
	d.memory = memory
	d.platform = platform
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
