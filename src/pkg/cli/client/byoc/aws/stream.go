package aws

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/cw"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	"github.com/DefangLabs/defang/src/pkg/types"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var codeBuildPrefixRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+-image/`)

// logEventParser converts raw CW log events into TailResponse protos.
type logEventParser struct {
	etag     string
	services []string
}

func (p *logEventParser) parseEvent(event cw.LogEvent) *defangv1.TailResponse {
	return p.parseEvents([]cw.LogEvent{event})
}

func (p *logEventParser) parseEvents(events []cw.LogEvent) *defangv1.TailResponse {
	if len(events) == 0 {
		return nil
	}

	var response defangv1.TailResponse
	parseFirelensRecords := false
	parseECSEventRecords := false
	parseCodeBuildRecords := false
	// Get the Etag/Host/Service from the first entry (should be the same for all events in this batch)
	first := events[0]
	switch {
	case first.LogGroupIdentifier == nil || first.LogStreamName == nil:
		response.Service = "alb"
		// response.Host = TODO: we can get the ALB IP from the bucket object name
	case strings.HasSuffix(*first.LogGroupIdentifier, "/ecs"):
		// ECS lifecycle events. LogStreams: "f0b805a8-fa74-3212-b6ce-a981c011d337"
		parseECSEventRecords = true
	case strings.Contains(*first.LogGroupIdentifier, ":"+byoc.CdTaskPrefix):
		// These events are from the CD task: "crun/main/<taskID>" stream; we should detect stdout/stderr
		// LogStreams: "crun/main/0f2a8ccde0374239bdd04f5e07d8c523"
		response.Host = "pulumi"
		response.Service = "cd"
	case strings.HasSuffix(*first.LogGroupIdentifier, "/builds") && codeBuildPrefixRegex.MatchString(*first.LogStreamName):
		response.Host = "codebuild"
		response.Service = "cd"
		parseCodeBuildRecords = true
		if parts := strings.Split(*first.LogStreamName, "/"); len(parts) == 3 {
			// These events are from codebuild build: "<service>-image/<service>_<etag>/<build_id>" stream
			// LogStreams: "worker-image/worker_iw7wua572g4j/db0fa3d3-0bbd-4770-8db4-f036a944af13"
			response.Host = parts[2] // build id
			underscore := strings.LastIndexByte(parts[1], '_')
			response.Etag = parts[1][underscore+1:]
			response.Service = parts[0] // Use <service>-image as service name for build logs
		}
	case strings.Contains(*first.LogStreamName, "-firelens-"):
		// These events are from the Firelens sidecar "<service>/<kaniko>-firelens-<taskID>"; try to parse the JSON
		// or ""
		// LogStreams: "app-image/kaniko-firelens-babe6cdb246b4c10b5b7093bb294e6c7"
		var record logs.FirelensMessage
		if err := json.Unmarshal([]byte(*first.Message), &record); err == nil {
			response.Etag = record.Etag
			response.Host = record.Host
			response.Service = record.ContainerName // TODO: ContainerName could be service_etag
			parseFirelensRecords = true
			break
		}
		fallthrough
	default:
		if parts := strings.Split(*first.LogStreamName, "/"); len(parts) == 3 {
			// These events are from an awslogs ECS task: "<tenant>/<service>_<etag>/<taskID>" stream
			// LogStreams: "app/app_hg2xsgvsldqk/198f58c08c734bda924edc516f93b2d5"
			response.Host = parts[2] // TODO: figure out actual hostname/IP for Task ID
			underscore := strings.LastIndexByte(parts[1], '_')
			etag, err := types.ParseEtag(parts[1][underscore+1:])
			if err == nil {
				response.Service = parts[1][:underscore]
				response.Etag = etag
				break
			}
		}
		term.Debugf("unrecognized log stream format: %s", *first.LogStreamName)
		return nil // skip, ignore sidecar logs (like route53-sidecar or fluentbit)
	}

	// Client-side filtering on etag and service (if provided)
	if response.Etag != "" && p.etag != "" && p.etag != response.Etag {
		return nil // TODO: filter these out using the AWS StartLiveTail API
	}
	if len(p.services) > 0 && !slices.Contains(p.services, response.GetService()) {
		return nil // TODO: filter these out using the AWS StartLiveTail API
	}

	entries := make([]*defangv1.LogEntry, 0, len(events))
	for _, event := range events {
		entry := &defangv1.LogEntry{
			Message:   *event.Message,
			Stderr:    false,
			Timestamp: timestamppb.New(time.UnixMilli(*event.Timestamp)),
		}
		if parseFirelensRecords {
			var record logs.FirelensMessage
			if err := json.Unmarshal([]byte(entry.Message), &record); err == nil {
				entry.Message = record.Log
				if record.ContainerName == "kaniko" {
					entry.Service = record.Service
					entry.Stderr = logs.IsLogrusError(entry.Message)
				} else {
					entry.Stderr = record.Source == logs.SourceStderr
				}
			}
		} else if parseECSEventRecords {
			evt, err := ecs.ParseECSEvent([]byte(*event.Message))
			if err != nil {
				term.Debugf("error parsing ECS event, output raw event log: %v", err)
			} else {
				entry.Service = evt.Service()
				entry.Etag = evt.Etag()
				entry.Host = evt.Host()
				entry.Message = evt.Status()
			}
		} else if parseCodeBuildRecords {
			entry.Service = response.Service
			entry.Etag = response.Etag
			entry.Host = response.Host
		} else if (response.Service == "cd") && (strings.HasPrefix(entry.Message, logs.ErrorPrefix) || strings.Contains(strings.ToLower(entry.Message), "error:")) {
			entry.Stderr = true
		}
		if entry.Etag != "" && p.etag != "" && entry.Etag != p.etag {
			continue
		}
		if entry.Service != "" && len(p.services) > 0 && !slices.Contains(p.services, entry.Service) {
			continue
		}

		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}
	response.Entries = entries
	return &response
}
