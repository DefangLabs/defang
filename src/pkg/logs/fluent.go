package logs

const (
	SourceStdout = "stdout"
	SourceStderr = "stderr"
)

type FirelensMessage struct {
	Log               string `json:"log"`
	ContainerID       string `json:"container_id,omitempty"`
	ContainerName     string `json:"container_name,omitempty"`
	Source            string `json:"source,omitempty"`              // "stdout" or "stderr"
	EcsTaskDefinition string `json:"ecs_task_definition,omitempty"` // ECS metadata
	EcsTaskArn        string `json:"ecs_task_arn,omitempty"`        // ECS metadata
	EcsCluster        string `json:"ecs_cluster,omitempty"`         // ECS metadata
	Tag               string `json:"tag,omitempty"`                 // added by NATS output filter
	Etag              string `json:"etag,omitempty"`                // added by us
	Host              string `json:"host,omitempty"`                // added by us
	Service           string `json:"service,omitempty"`             // added by us (for label); TODO: deprecated
	Tenant            string `json:"tenant,omitempty"`              // added by us; TODO: deprecated
}
