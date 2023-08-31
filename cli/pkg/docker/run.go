package docker

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func (d Docker) Run(ctx context.Context, env map[string]string, cmd ...string) (ContainerID, error) {
	resp, err := d.ContainerCreate(ctx, &container.Config{
		Image: d.image,
		Env:   mapToSlice(env),
		Cmd:   cmd,
	}, nil, nil, nil, "")
	if err != nil {
		return nil, err
	}

	return &resp.ID, d.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
}

func mapToSlice(m map[string]string) []string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		// Ensure no = in key
		if strings.ContainsRune(k, '=') {
			panic("invalid environment variable key")
		}
		s = append(s, k+"="+v)
	}
	return s
}
