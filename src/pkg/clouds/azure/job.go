package azure

import (
	"context"

	"github.com/DefangLabs/defang/src/pkg/types"
)

type ContainerJob struct {
}

// var _ types.Driver = (*ContainerJob)(nil)

// CreateUploadURL implements types.Driver.
func (c *ContainerJob) CreateUploadURL(ctx context.Context, name string) (string, error) {
	panic("unimplemented")
}

// GetInfo implements types.Driver.
func (c *ContainerJob) GetInfo(ctx context.Context, taskID types.TaskID) (*types.TaskInfo, error) {
	panic("unimplemented")
}

// ListSecrets implements types.Driver.
func (c *ContainerJob) ListSecrets(ctx context.Context) ([]string, error) {
	panic("unimplemented")
}

// PutSecret implements types.Driver.
func (c *ContainerJob) PutSecret(ctx context.Context, name string, value string) error {
	panic("unimplemented")
}
