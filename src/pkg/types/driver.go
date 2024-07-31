package types

import (
	"context"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

const (
	ProjectName = "crun"
)

type TaskID *string
type ContainerCondition string

const (
	ContainerStarted  ContainerCondition = "START"
	ContainerComplete                    = "COMPLETE"
	ContainerSuccess                     = "SUCCESS"
	ContainerHealthy                     = "HEALTHY"
)

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
	WorkDir     *string
	DependsOn   map[string]ContainerCondition // container name -> condition
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
	PutConfig(ctx context.Context, name, value string, isSensitive bool) error
	GetConfig(ctx context.Context, name []string, rootPath string) (*defangv1.ConfigValues, error)
	// DeleteSecrets(ctx context.Context, names ...string) error
	ListConfigs(ctx context.Context) ([]string, error) // no values
	CreateUploadURL(ctx context.Context, name string) (string, error)
}

type TaskInfo struct {
	IP string
}
