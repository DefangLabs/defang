package codebuild

import (
	"time"

	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
)

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

type CodebuildEvent struct {
	eventCommonFields
}

func ParseCodebuildEvent(entry *defangv1.LogEntry) Event {
	// TODO: implement parsing of CodeBuild events from log entries
	return &CodebuildEvent{}
}

func (e *CodebuildEvent) State() defangv1.ServiceState {
	// TODO: implement mapping of CodeBuild event details to ServiceState
	return defangv1.ServiceState_NOT_SPECIFIED
}

func (e *CodebuildEvent) Service() string {
	// TODO: implement extraction of service name from CodeBuild event details
	return ""
}

func (e *CodebuildEvent) Etag() string {
	// TODO: implement extraction of etag from CodeBuild event details
	return ""
}

func (e *CodebuildEvent) Host() string {
	return "codebuild"
}

func (e *CodebuildEvent) Status() string {
	// TODO: implement extraction of status from CodeBuild event details
	return ""
}
