package docker

import (
	"context"
	"errors"

	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"github.com/docker/docker/client"
)

type ContainerID = types.TaskID

type Docker struct {
	*client.Client

	image    string
	memory   uint64
	platform string
}

func New() *Docker {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	return &Docker{
		Client: cli,
	}
}

var _ types.Driver = (*Docker)(nil)

func (Docker) PutConfig(ctx context.Context, rootPath, name, value string, isSensitive bool) error {
	return errors.New("docker does not support secrets")
}

func (Docker) GetConfigs(ctx context.Context, rootPath string, names ...string) (*defangv1.GetConfigsResponse, error) {
	return nil, errors.New("docker does not support secrets")
}

func (Docker) DeleteConfigs(ctx context.Context, rootPath string, name ...string) error {
	return errors.New("docker does not support secrets")
}

func (Docker) ListConfigs(ctx context.Context, projectName string) ([]string, error) {
	return nil, errors.New("docker does not support secrets")
}

func (Docker) CreateUploadURL(ctx context.Context, name string) (string, error) {
	return "", errors.New("docker does not support uploads")
}
