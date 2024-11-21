package logs

type Source string

const (
	SourceStdout Source = "stdout"
	SourceStderr Source = "stderr"

	ErrorPrefix = " ** "
)

type FirelensMessage struct {
	Log               string `json:"log"`
	ContainerID       string `json:"container_id,omitempty"`
	ContainerName     string `json:"container_name,omitempty"`
	Source            Source `json:"source,omitempty"`              // "stdout" or "stderr"
	EcsTaskDefinition string `json:"ecs_task_definition,omitempty"` // ECS metadata
	EcsTaskArn        string `json:"ecs_task_arn,omitempty"`        // ECS metadata
	EcsCluster        string `json:"ecs_cluster,omitempty"`         // ECS metadata
	Etag              string `json:"etag,omitempty"`                // added by us
	Host              string `json:"host,omitempty"`                // added by us
	Service           string `json:"service,omitempty"`             // added by us
}
