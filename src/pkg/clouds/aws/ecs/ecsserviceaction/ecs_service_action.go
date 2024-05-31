package ecsserviceaction

import (
    "time"
)


type ECSServiceAction struct {
    CapacityProviderArns []string `json:"capacityProviderArns,omitempty"`
    ClusterArn string `json:"clusterArn"`
    CreatedAt time.Time `json:"createdAt,omitempty"`
    EventName string `json:"eventName"`
    EventType string `json:"eventType"`
    Reason string `json:"reason,omitempty"`
    DesiredCount float64 `json:"desiredCount,omitempty"`
    ContainerPort float64 `json:"containerPort,omitempty"`
    TaskArns []string `json:"taskArns,omitempty"`
    TaskSetArns []string `json:"taskSetArns,omitempty"`
    ContainerInstanceArns []string `json:"containerInstanceArns,omitempty"`
    Ec2InstanceIds []string `json:"ec2InstanceIds,omitempty"`
    TargetGroupArns []string `json:"targetGroupArns,omitempty"`
    ServiceRegistryArns []string `json:"serviceRegistryArns,omitempty"`
    Targets []string `json:"targets,omitempty"`
}

func (e *ECSServiceAction) SetCapacityProviderArns(capacityProviderArns []string) {
    e.CapacityProviderArns = capacityProviderArns
}

func (e *ECSServiceAction) SetClusterArn(clusterArn string) {
    e.ClusterArn = clusterArn
}

func (e *ECSServiceAction) SetCreatedAt(createdAt time.Time) {
    e.CreatedAt = createdAt
}

func (e *ECSServiceAction) SetEventName(eventName string) {
    e.EventName = eventName
}

func (e *ECSServiceAction) SetEventType(eventType string) {
    e.EventType = eventType
}

func (e *ECSServiceAction) SetReason(reason string) {
    e.Reason = reason
}

func (e *ECSServiceAction) SetDesiredCount(desiredCount float64) {
    e.DesiredCount = desiredCount
}

func (e *ECSServiceAction) SetContainerPort(containerPort float64) {
    e.ContainerPort = containerPort
}

func (e *ECSServiceAction) SetTaskArns(taskArns []string) {
    e.TaskArns = taskArns
}

func (e *ECSServiceAction) SetTaskSetArns(taskSetArns []string) {
    e.TaskSetArns = taskSetArns
}

func (e *ECSServiceAction) SetContainerInstanceArns(containerInstanceArns []string) {
    e.ContainerInstanceArns = containerInstanceArns
}

func (e *ECSServiceAction) SetEc2InstanceIds(ec2InstanceIds []string) {
    e.Ec2InstanceIds = ec2InstanceIds
}

func (e *ECSServiceAction) SetTargetGroupArns(targetGroupArns []string) {
    e.TargetGroupArns = targetGroupArns
}

func (e *ECSServiceAction) SetServiceRegistryArns(serviceRegistryArns []string) {
    e.ServiceRegistryArns = serviceRegistryArns
}

func (e *ECSServiceAction) SetTargets(targets []string) {
    e.Targets = targets
}
