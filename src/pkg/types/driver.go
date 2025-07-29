package types

import (
	"context"
)

const (
	ProjectName = "crun"
)

type TaskID *string

type Container struct {
	Image    string
	Name     string
	Cpus     float64
	Memory   uint64
	Platform string
	IsInit   bool // whether this container is an init container (non-essential)

	Volumes     []TaskVolume
	VolumesFrom []string // "container" (default rw), "container:rw", or "container:ro"
	EntryPoint  []string
	Command     []string // overridden by Run()
	WorkDir     string   // Deprecated: not supported by ACI
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
	GetInfo(ctx context.Context, taskID TaskID) (*TaskInfo, error)
	PutSecret(ctx context.Context, name, value string) error
	// DeleteSecrets(ctx context.Context, names ...string) error
	ListSecrets(ctx context.Context) ([]string, error) // no values
	CreateUploadURL(ctx context.Context, name string) (string, error)
}

type TaskInfo struct {
	IP string
}
