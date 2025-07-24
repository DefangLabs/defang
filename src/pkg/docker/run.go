package docker

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

func (d Docker) Run(ctx context.Context, env map[string]string, cmd ...string) (ContainerID, error) {
	resp, err := d.ContainerCreate(ctx, &container.Config{
		Image: d.image,
		Env:   mapToSlice(env),
		Cmd:   cmd,
	}, &container.HostConfig{
		AutoRemove:      true, // --rm; FIXME: this causes "No such container" if the container exits early
		PublishAllPorts: true, // -P
		Resources: container.Resources{
			Memory: int64(d.memory), // #nosec G115 - memory is expected to be a small number
		},
	}, nil, parsePlatform(d.platform), "")
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

func parsePlatform(platform string) *v1.Platform {
	parts := strings.Split(platform, "/")
	var p = &v1.Platform{}
	switch len(parts) {
	case 3:
		p.Variant = parts[2]
		fallthrough
	case 2:
		p.OS = parts[0]
		p.Architecture = parts[1]
	case 1:
		p.Architecture = parts[0]
	default:
		panic("invalid platform: " + platform)
	}
	return p
}
