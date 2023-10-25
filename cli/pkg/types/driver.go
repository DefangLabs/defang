package types

import (
	"context"
)

const (
	ProjectName = "crun"
)

type TaskID *string

type Driver interface {
	SetUp(ctx context.Context, image string, memory uint64, platform string) error
	TearDown(ctx context.Context) error
	Run(ctx context.Context, env map[string]string, args ...string) (TaskID, error)
	Info(ctx context.Context, taskID TaskID) (string, error) // TODO: make return value a struct
	Tail(ctx context.Context, taskID TaskID) error
	Stop(ctx context.Context, taskID TaskID) error
	// Exec(ctx context.Context, taskID TaskID, args ...string) error
}
