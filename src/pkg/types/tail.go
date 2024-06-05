package types

import (
	"time"
)

type EndLogConditional struct {
	Service  string
	Host     string
	EventLog string
}

type TailDetectStopEventFunc func(service string, host string, eventlog string) bool

type TailOptions struct {
	Service            string
	Etag               ETag
	Since              time.Time
	Raw                bool
	EndEventDetectFunc TailDetectStopEventFunc
}
