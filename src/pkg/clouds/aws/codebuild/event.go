package codebuild

import (
	"strings"
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
	Account    string
	DetailType string
	Id         string
	Region     string
	Resources  []string
	Source     string
	Time       time.Time
	Version    string
}

type CodebuildEvent struct {
	eventCommonFields
	message string
	service string
	etag    string
	host    string
}

func ParseCodebuildEvent(entry *defangv1.LogEntry) Event {
	message := entry.Message
	return &CodebuildEvent{
		message: message,
		service: entry.Service,
		etag:    entry.Etag,
		host:    entry.Host,
	}
}

func (e *CodebuildEvent) State() defangv1.ServiceState {
	if strings.Contains(e.message, "Phase complete: ") && strings.Contains(e.message, "State: FAILED") {
		return defangv1.ServiceState_BUILD_FAILED
	}
	if strings.Contains(e.message, "Running on CodeBuild") {
		return defangv1.ServiceState_BUILD_ACTIVATING
	}
	if strings.Contains(e.message, "Phase is DOWNLOAD_SOURCE") {
		return defangv1.ServiceState_BUILD_RUNNING
	}

	if strings.Contains(e.message, "Entering phase INSTALL") {
		return defangv1.ServiceState_BUILD_RUNNING
	}

	if strings.Contains(e.message, "Entering phase PRE_BUILD") {
		return defangv1.ServiceState_BUILD_RUNNING
	}

	if strings.Contains(e.message, "Entering phase BUILD") {
		return defangv1.ServiceState_BUILD_RUNNING
	}

	if strings.Contains(e.message, "Entering phase POST_BUILD") {
		return defangv1.ServiceState_BUILD_STOPPING
	}

	if strings.Contains(e.message, "Phase complete: UPLOAD_ARTIFACTS State: SUCCEEDED") {
		return defangv1.ServiceState_DEPLOYMENT_PENDING
	}

	return defangv1.ServiceState_NOT_SPECIFIED
}

func (e *CodebuildEvent) Service() string {
	return e.service
}

func (e *CodebuildEvent) Etag() string {
	return e.etag
}

func (e *CodebuildEvent) Host() string {
	return "codebuild"
}

func (e *CodebuildEvent) Status() string {
	return ""
}
