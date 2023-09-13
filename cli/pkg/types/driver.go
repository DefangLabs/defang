package types

import (
	"context"
)

const (
	ProjectName = "crun"
)

type TaskID *string

type Driver interface {
	SetUp(ctx context.Context, image string, memory uint64) error
	TearDown(ctx context.Context) error
	Run(ctx context.Context, env map[string]string, args ...string) (TaskID, error)
	Tail(ctx context.Context, taskID TaskID) error
	Stop(ctx context.Context, taskID TaskID) error
}
