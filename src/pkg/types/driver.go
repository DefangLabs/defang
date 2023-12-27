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
	Tail(ctx context.Context, taskID TaskID) error
	Stop(ctx context.Context, taskID TaskID) error
	// Exec(ctx context.Context, taskID TaskID, args ...string) error
	GetInfo(ctx context.Context, taskID TaskID) (string, error) // TODO: make return value a struct
	SetVpcID(vpcId string) error
	PutSecret(ctx context.Context, name, value string) error
	ListSecrets(ctx context.Context) ([]string, error) // no values
}
