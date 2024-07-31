package ecstaskstatechange

import (
    "time"
)


type ECSTaskStateChange struct {
    Overrides Overrides `json:"overrides"`
    ExecutionStoppedAt time.Time `json:"executionStoppedAt,omitempty"`
    Memory string `json:"memory,omitempty"`
    Attachments []AttachmentDetails `json:"attachments,omitempty"`
    Attributes []AttributesDetails `json:"attributes,omitempty"`
    PullStartedAt time.Time `json:"pullStartedAt,omitempty"`
    TaskArn string `json:"taskArn"`
    StartedAt time.Time `json:"startedAt,omitempty"`
    CreatedAt time.Time `json:"createdAt"`
    ClusterArn string `json:"clusterArn"`
    Connectivity string `json:"connectivity,omitempty"`
    PlatformVersion string `json:"platformVersion,omitempty"`
    ContainerInstanceArn string `json:"containerInstanceArn,omitempty"`
    LaunchType string `json:"launchType,omitempty"`
    Group string `json:"group,omitempty"`
    UpdatedAt time.Time `json:"updatedAt"`
    StopCode string `json:"stopCode,omitempty"`
    PullStoppedAt time.Time `json:"pullStoppedAt,omitempty"`
    ConnectivityAt time.Time `json:"connectivityAt,omitempty"`
    StartedBy string `json:"startedBy,omitempty"`
    Cpu string `json:"cpu,omitempty"`
    Version float64 `json:"version"`
    StoppingAt time.Time `json:"stoppingAt,omitempty"`
    StoppedAt time.Time `json:"stoppedAt,omitempty"`
    TaskDefinitionArn string `json:"taskDefinitionArn"`
    StoppedReason string `json:"stoppedReason,omitempty"`
    Containers []ContainerDetails `json:"containers"`
    DesiredStatus string `json:"desiredStatus"`
    LastStatus string `json:"lastStatus"`
    AvailabilityZone string `json:"availabilityZone,omitempty"`
}

func (e *ECSTaskStateChange) SetOverrides(overrides Overrides) {
    e.Overrides = overrides
}

func (e *ECSTaskStateChange) SetExecutionStoppedAt(executionStoppedAt time.Time) {
    e.ExecutionStoppedAt = executionStoppedAt
}

func (e *ECSTaskStateChange) SetMemory(memory string) {
    e.Memory = memory
}

func (e *ECSTaskStateChange) SetAttachments(attachments []AttachmentDetails) {
    e.Attachments = attachments
}

func (e *ECSTaskStateChange) SetAttributes(attributes []AttributesDetails) {
    e.Attributes = attributes
}

func (e *ECSTaskStateChange) SetPullStartedAt(pullStartedAt time.Time) {
    e.PullStartedAt = pullStartedAt
}

func (e *ECSTaskStateChange) SetTaskArn(taskArn string) {
    e.TaskArn = taskArn
}

func (e *ECSTaskStateChange) SetStartedAt(startedAt time.Time) {
    e.StartedAt = startedAt
}

func (e *ECSTaskStateChange) SetCreatedAt(createdAt time.Time) {
    e.CreatedAt = createdAt
}

func (e *ECSTaskStateChange) SetClusterArn(clusterArn string) {
    e.ClusterArn = clusterArn
}

func (e *ECSTaskStateChange) SetConnectivity(connectivity string) {
    e.Connectivity = connectivity
}

func (e *ECSTaskStateChange) SetPlatformVersion(platformVersion string) {
    e.PlatformVersion = platformVersion
}

func (e *ECSTaskStateChange) SetContainerInstanceArn(containerInstanceArn string) {
    e.ContainerInstanceArn = containerInstanceArn
}

func (e *ECSTaskStateChange) SetLaunchType(launchType string) {
    e.LaunchType = launchType
}

func (e *ECSTaskStateChange) SetGroup(group string) {
    e.Group = group
}

func (e *ECSTaskStateChange) SetUpdatedAt(updatedAt time.Time) {
    e.UpdatedAt = updatedAt
}

func (e *ECSTaskStateChange) SetStopCode(stopCode string) {
    e.StopCode = stopCode
}

func (e *ECSTaskStateChange) SetPullStoppedAt(pullStoppedAt time.Time) {
    e.PullStoppedAt = pullStoppedAt
}

func (e *ECSTaskStateChange) SetConnectivityAt(connectivityAt time.Time) {
    e.ConnectivityAt = connectivityAt
}

func (e *ECSTaskStateChange) SetStartedBy(startedBy string) {
    e.StartedBy = startedBy
}

func (e *ECSTaskStateChange) SetCpu(cpu string) {
    e.Cpu = cpu
}

func (e *ECSTaskStateChange) SetVersion(version float64) {
    e.Version = version
}

func (e *ECSTaskStateChange) SetStoppingAt(stoppingAt time.Time) {
    e.StoppingAt = stoppingAt
}

func (e *ECSTaskStateChange) SetStoppedAt(stoppedAt time.Time) {
    e.StoppedAt = stoppedAt
}

func (e *ECSTaskStateChange) SetTaskDefinitionArn(taskDefinitionArn string) {
    e.TaskDefinitionArn = taskDefinitionArn
}

func (e *ECSTaskStateChange) SetStoppedReason(stoppedReason string) {
    e.StoppedReason = stoppedReason
}

func (e *ECSTaskStateChange) SetContainers(containers []ContainerDetails) {
    e.Containers = containers
}

func (e *ECSTaskStateChange) SetDesiredStatus(desiredStatus string) {
    e.DesiredStatus = desiredStatus
}

func (e *ECSTaskStateChange) SetLastStatus(lastStatus string) {
    e.LastStatus = lastStatus
}

func (e *ECSTaskStateChange) SetAvailabilityZone(availabilityZone string) {
    e.AvailabilityZone = availabilityZone
}
