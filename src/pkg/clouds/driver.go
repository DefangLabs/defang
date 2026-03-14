package clouds

const (
	ProjectName = "crun"
)

type TaskID *string
type ContainerCondition string

const (
	ContainerStarted ContainerCondition = "START"
	ContainerSuccess ContainerCondition = "SUCCESS"
	ContainerHealthy ContainerCondition = "HEALTHY"
)

type Container struct {
	Image       string
	Name        string
	Cpus        float32
	Memory      uint64
	Platform    string
	Essential   *bool // default true
	Volumes     []TaskVolume
	VolumesFrom []string // container (default rw), container:rw, or container:ro
	EntryPoint  []string
	Command     []string // overridden by Run()
	WorkDir     string
	DependsOn   map[string]ContainerCondition // container name -> condition
}

type TaskVolume struct {
	Source   string
	Target   string
	ReadOnly bool
}
