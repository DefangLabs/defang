package types

import (
	"context"
)

const (
	ProjectName = "crun"
)

type TaskID *string

type Container struct {
	Image       string
	Name        string
	Cpus        float32
	Memory      uint64
	Platform    string
	Essential   *bool
	Volumes     []TaskVolume
	VolumesFrom []string // container (default rw), container:rw, or container:ro
	EntryPoint  []string
	Command     []string // overridden by Run()
}

type TaskVolume struct {
	Source   string
	Target   string
	ReadOnly bool
}

type Driver interface {
	SetUp(ctx context.Context, containers []Container) error
	TearDown(ctx context.Context) error
	Run(ctx context.Context, env map[string]string, args ...string) (TaskID, error)
	Tail(ctx context.Context, taskID TaskID) error
	// Query(ctx context.Context, taskID TaskID, since time.Time) error
	Stop(ctx context.Context, taskID TaskID) error
	// Exec(ctx context.Context, taskID TaskID, args ...string) error
	GetInfo(ctx context.Context, taskID TaskID) (string, error) // TODO: make return value a struct
	SetVpcID(vpcId string) error
	PutSecret(ctx context.Context, name, value string) error
	ListSecrets(ctx context.Context) ([]string, error) // no values
	CreateUploadURL(ctx context.Context, name string) (string, error)
}
