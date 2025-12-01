package ecs

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/ecsserviceaction"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/ecstaskstatechange"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

type Cache interface {
	Get(string) string
	Set(string, string)
}

type LocalCache map[string]string

var DeploymentEtags Cache = make(LocalCache)

func (c LocalCache) Get(k string) string {
	return c[k]
}

func (c LocalCache) Set(k, v string) {
	c[k] = v
}

type ECSServiceAction = ecsserviceaction.ECSServiceAction

type ECSTaskStateChange = ecstaskstatechange.ECSTaskStateChange

type ECSDeploymentStateChange struct {
	ECSServiceAction
	DeploymentId string `json:"deploymentId,omitempty"`
}

type Event interface {
	Service() string
	Etag() string
	Host() string
	Status() string
	State() defangv1.ServiceState
}

type eventCommonFields struct {
	Account    string    `json:"account"`
	DetailType string    `json:"detail-type"`
	Id         string    `json:"id"`
	Region     string    `json:"region"`
	Resources  []string  `json:"resources"`
	Source     string    `json:"source"`
	Time       time.Time `json:"time"`
	Version    string    `json:"version"`
}

type TaskStateChangeEvent struct {
	eventCommonFields
	Detail ECSTaskStateChange
}

type KanikoTaskStateChangeEvent TaskStateChangeEvent

type ServiceActionEvent struct {
	eventCommonFields
	Detail ECSServiceAction
}

type DeploymentStateChangeEvent struct {
	eventCommonFields
	Detail ECSDeploymentStateChange
}

func ParseECSEvent(b []byte) (Event, error) {
	var e struct {
		eventCommonFields
		Detail json.RawMessage `json:"detail"`
	}
	if err := json.Unmarshal(b, &e); err != nil {
		return nil, err
	}
	switch e.DetailType {
	case "ECS Task State Change":
		var detail ECSTaskStateChange
		if err := json.Unmarshal(e.Detail, &detail); err != nil {
			return nil, err
		}

		var evt Event
		for _, o := range detail.Overrides.ContainerOverrides {
			if o.Name == "kaniko" {
				evt = &KanikoTaskStateChangeEvent{e.eventCommonFields, detail}
			}
		}
		if evt == nil {
			e := &TaskStateChangeEvent{e.eventCommonFields, detail}
			if len(e.Detail.Overrides.ContainerOverrides) == 0 {
				return nil, fmt.Errorf("no container overrides for task state change event: %v", e.Detail.TaskArn)
			}
			evt = e
		}

		etag := evt.Etag()
		deploymentId := detail.StartedBy
		if etag != "" && deploymentId != "" {
			DeploymentEtags.Set(deploymentId, etag)
		}

		return evt, nil
	case "ECS Service Action":
		var detail ECSServiceAction
		if err := json.Unmarshal(e.Detail, &detail); err != nil {
			return nil, err
		}
		return &ServiceActionEvent{e.eventCommonFields, detail}, nil
	case "ECS Deployment State Change":
		var detail ECSDeploymentStateChange
		if err := json.Unmarshal(e.Detail, &detail); err != nil {
			return nil, err
		}
		return &DeploymentStateChangeEvent{e.eventCommonFields, detail}, nil
	default:
		return nil, fmt.Errorf("unsupported ECS event type: %s", e.DetailType)
	}
}

func (e *TaskStateChangeEvent) Service() string {
	var service string
	override := e.Detail.Overrides.ContainerOverrides[0]
	i := strings.LastIndex(override.Name, "_")
	if i > 0 {
		service = override.Name[:i]
	}
	return service
}
func (e *TaskStateChangeEvent) Etag() string {
	var etag string
	override := e.Detail.Overrides.ContainerOverrides[0]
	i := strings.LastIndex(override.Name, "_")
	if i > 0 {
		etag = override.Name[i+1:]
	}
	return etag
}
func (e *TaskStateChangeEvent) Host() string {
	return "ecs"
}

func (e *TaskStateChangeEvent) Status() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "TASK_%s", e.Detail.LastStatus)
	if e.Detail.StoppedReason != "" {
		fmt.Fprintf(&buf, " %s", e.Detail.StoppedReason)
	}
	fmt.Fprintf(&buf, " : %s", path.Base(e.Detail.TaskArn))
	if e.Detail.LastStatus == "DEPROVISIONING" {
		exitCode := getTaskExitCode(e.Detail.Overrides.ContainerOverrides[0].Name, e.Detail.Containers)
		if exitCode != 0 {
			fmt.Fprintf(&buf, " (exit code %v)", exitCode)
		}
	}
	return buf.String()
}

func (e *TaskStateChangeEvent) State() defangv1.ServiceState {
	state := defangv1.ServiceState_NOT_SPECIFIED
	if e.Detail.LastStatus == "DEACTIVATING" {
		state = defangv1.ServiceState_DEPLOYMENT_FAILED // Fast deployment fail
	} else if e.Detail.LastStatus == "DEPROVISIONING" {
		exitCode := getTaskExitCode(e.Detail.Overrides.ContainerOverrides[0].Name, e.Detail.Containers)
		if exitCode != 0 {
			state = defangv1.ServiceState_DEPLOYMENT_FAILED
		}
	} else {
		state = defangv1.ServiceState_DEPLOYMENT_PENDING // Treat all other task updates as deployment pending
	}
	return state
}

func (e *KanikoTaskStateChangeEvent) Service() string {
	override := e.getKanikoOverride()
	if override == nil {
		return ""
	}

	var service string
	const prefix = `--label=io.defang.image.name=`
	const suffix = `-image`
	for _, arg := range override.Command {
		if strings.HasPrefix(arg, prefix) && strings.HasSuffix(arg, suffix) {
			service = strings.TrimSuffix(strings.TrimPrefix(arg, prefix), suffix)
			break
		}
	}
	return service
}

func (e *KanikoTaskStateChangeEvent) Etag() string {
	override := e.getKanikoOverride()
	if override == nil {
		return ""
	}

	var etag string
	const prefix = `--label=io.defang.image.etag=`
	for _, arg := range override.Command {
		if strings.HasPrefix(arg, prefix) {
			etag = strings.TrimPrefix(arg, prefix)
			break
		}
	}
	return etag
}
func (e *KanikoTaskStateChangeEvent) Host() string {
	return "kaniko"
}

func (e *KanikoTaskStateChangeEvent) Status() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "BUILD_%s", e.Detail.LastStatus)
	if e.Detail.StoppedReason != "" {
		fmt.Fprintf(&buf, " : %s", e.Detail.StoppedReason)
	}
	fmt.Fprintf(&buf, " : %s", path.Base(e.Detail.TaskArn))
	override := e.getKanikoOverride()
	if override != nil {
		exitCode := getTaskExitCode(override.Name, e.Detail.Containers)
		if exitCode != 0 {
			fmt.Fprintf(&buf, " (exit code %v)", exitCode)
		}
	}
	return buf.String()
}

var kanikoTaskStateMap = map[string]defangv1.ServiceState{
	"PROVISIONING":   defangv1.ServiceState_BUILD_PROVISIONING,
	"PENDING":        defangv1.ServiceState_BUILD_PENDING,
	"ACTIVATING":     defangv1.ServiceState_BUILD_ACTIVATING,
	"RUNNING":        defangv1.ServiceState_BUILD_RUNNING,
	"DEPROVISIONING": defangv1.ServiceState_BUILD_STOPPING,
	// "STOPPED":        defangv1.ServiceState_BUILD_STOPPING, // Ignored
}

func (e *KanikoTaskStateChangeEvent) State() defangv1.ServiceState {
	override := e.getKanikoOverride()
	if override == nil {
		return defangv1.ServiceState_NOT_SPECIFIED
	}

	state := kanikoTaskStateMap[e.Detail.LastStatus]
	if state == defangv1.ServiceState_BUILD_STOPPING {
		exitCode := getTaskExitCode(override.Name, e.Detail.Containers)
		if exitCode != 0 {
			state = defangv1.ServiceState_BUILD_FAILED
		}
	}
	return state
}

func (e *KanikoTaskStateChangeEvent) getKanikoOverride() *ecstaskstatechange.OverridesItem {
	for _, c := range e.Detail.Overrides.ContainerOverrides {
		if c.Name == "kaniko" {
			return &c
		}
	}
	return nil
}

func (e *ServiceActionEvent) Service() string {
	return serviceNameFromResources(e.Resources)
}
func (e *ServiceActionEvent) Etag() string {
	return ""
}
func (e *ServiceActionEvent) Host() string {
	return "ecs"
}
func (e *ServiceActionEvent) Status() string {
	return e.Detail.EventName
}
func (e *ServiceActionEvent) State() defangv1.ServiceState {
	return defangv1.ServiceState_NOT_SPECIFIED
}

func (e *DeploymentStateChangeEvent) Service() string {
	return serviceNameFromResources(e.Resources)
}
func (e *DeploymentStateChangeEvent) Etag() string {
	return DeploymentEtags.Get(e.Detail.DeploymentId)
}
func (e *DeploymentStateChangeEvent) Host() string {
	return "ecs"
}
func (e *DeploymentStateChangeEvent) Status() string {
	return e.Detail.EventName
}

func (e *DeploymentStateChangeEvent) State() defangv1.ServiceState {
	state := defangv1.ServiceState_NOT_SPECIFIED
	switch e.Detail.EventName {
	case "SERVICE_DEPLOYMENT_IN_PROGRESS":
		state = defangv1.ServiceState_DEPLOYMENT_PENDING
	case "SERVICE_DEPLOYMENT_COMPLETED":
		state = defangv1.ServiceState_DEPLOYMENT_COMPLETED
	case "SERVICE_DEPLOYMENT_FAILED":
		state = defangv1.ServiceState_DEPLOYMENT_FAILED
	}
	return state
}

func serviceNameFromResources(resources []string) string {
	if len(resources) <= 0 {
		return ""
	}
	id := path.Base(resources[0]) // project_service-random
	snStart := strings.Index(id, "_")
	snEnd := strings.LastIndex(id, "-")
	if snStart < 0 || snEnd < 0 || snStart >= snEnd {
		return ""
	}
	return id[snStart+1 : snEnd]
}

func getTaskExitCode(name string, containers []ecstaskstatechange.ContainerDetails) float64 {
	for _, c := range containers {
		if c.Name == name {
			return c.ExitCode
		}
	}
	return 0
}
