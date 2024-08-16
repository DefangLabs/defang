package ecs

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/ecsserviceaction"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs/ecstaskstatechange"
)

type ECSServiceAction = ecsserviceaction.ECSServiceAction

type ECSTaskStateChange = ecstaskstatechange.ECSTaskStateChange

type ECSDeploymentStateChange struct {
	ECSServiceAction
	DeploymentId string `json:"deploymentId,omitempty"`
}

type Event struct {
	Detail     json.RawMessage `json:"detail"`
	Account    string          `json:"account"`
	DetailType string          `json:"detail-type"`
	Id         string          `json:"id"`
	Region     string          `json:"region"`
	Resources  []string        `json:"resources"`
	Source     string          `json:"source"`
	Time       time.Time       `json:"time"`
	Version    string          `json:"version"`
}

func FindContainerOverrides(detail *ECSTaskStateChange, containerName string) *ecstaskstatechange.OverridesItem {
	for _, co := range detail.Overrides.ContainerOverrides {
		if strings.HasPrefix(co.Name, containerName) {
			return &co
		}
	}
	return nil
}
