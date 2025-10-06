package aws

import (
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/DefangLabs/defang/src/pkg"
	"github.com/DefangLabs/defang/src/pkg/cli/client"
	"github.com/DefangLabs/defang/src/pkg/cli/client/byoc"
	"github.com/DefangLabs/defang/src/pkg/clouds/aws/ecs"
	"github.com/DefangLabs/defang/src/pkg/logs"
	"github.com/DefangLabs/defang/src/pkg/term"
	defangv1 "github.com/DefangLabs/defang/src/protos/io/defang/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// byocServerStream is a wrapper around awsecs.EventStream that implements connect-like ServerStream
type byocServerStream struct {
	err      error
	etag     string
	response *defangv1.TailResponse
	services []string
	stream   ecs.LiveTailStream

	ecsEventsHandler ECSEventHandler
}

func newByocServerStream(stream ecs.LiveTailStream, etag string, services []string, ecsEventHandler ECSEventHandler) *byocServerStream {
	return &byocServerStream{
		etag:     etag,
		stream:   stream,
		services: services,

		ecsEventsHandler: ecsEventHandler,
	}
}

var _ client.ServerStream[defangv1.TailResponse] = (*byocServerStream)(nil)

func (bs *byocServerStream) Close() error {
	return bs.stream.Close()
}

func (bs *byocServerStream) Err() error {
	if bs.err == io.EOF {
		return nil // same as the original gRPC/connect server stream
	}
	return bs.err
}

func (bs *byocServerStream) Msg() *defangv1.TailResponse {
	return bs.response
}

func (bs *byocServerStream) Receive() bool {
	e := <-bs.stream.Events()
	if err := bs.stream.Err(); err != nil {
		bs.err = AnnotateAwsError(err)
		return false
	}
	evts, err := ecs.GetLogEvents(e)
	if err != nil {
		bs.err = err
		return false
	}
	bs.response = bs.parseEvents(evts)
	return true
}

func (bs *byocServerStream) parseEvents(events []ecs.LogEvent) *defangv1.TailResponse {
	if len(events) == 0 {
		// The original gRPC/connect server stream would never send an empty response.
		// We could loop around the select, but returning an empty response updates the spinner.
		return nil
	}
	var response defangv1.TailResponse
	parseFirelensRecords := false
	parseECSEventRecords := false
	// Get the Etag/Host/Service from the first entry (should be the same for all events in this batch)
	first := events[0]
	switch {
	case strings.HasSuffix(*first.LogGroupIdentifier, "/ecs"):
		// ECS lifecycle events. LogStreams: "f0b805a8-fa74-3212-b6ce-a981c011d337"
		parseECSEventRecords = true
	case strings.Contains(*first.LogGroupIdentifier, ":"+byoc.CdTaskPrefix):
		// These events are from the CD task: "crun/main/<taskID>" stream; we should detect stdout/stderr
		// LogStreams: "crun/main/0f2a8ccde0374239bdd04f5e07d8c523"
		response.Host = "pulumi"
		response.Service = "cd"
	case strings.HasSuffix(*first.LogGroupIdentifier, "/builds") && strings.Contains(*first.LogStreamName, "-firelens-"):
		// These events are from the Firelens sidecar "<service>/<kaniko>-firelens-<taskID>"; try to parse the JSON
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
			if etag := parts[1][underscore+1:]; pkg.IsValidRandomID(etag) {
				response.Service = parts[1][:underscore]
				response.Etag = etag
				break
			}
		}
		term.Debugf("unrecognized log stream format: %s", *first.LogStreamName)
		return nil // skip, ignore sidecar logs (like route53-sidecar or fluentbit)
	}

	// Client-side filtering on etag and service (if provided)
	if response.Etag != "" && bs.etag != "" && bs.etag != response.Etag {
		return nil // TODO: filter these out using the AWS StartLiveTail API
	}
	if len(bs.services) > 0 && !slices.Contains(bs.services, response.GetService()) {
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
				bs.ecsEventsHandler.HandleECSEvent(evt)
				entry.Service = evt.Service()
				entry.Etag = evt.Etag()
				entry.Host = evt.Host()
				entry.Message = evt.Status()
			}
		} else if response.Service == "cd" && (strings.HasPrefix(entry.Message, logs.ErrorPrefix) || strings.Contains(strings.ToLower(entry.Message), "error:")) {
			entry.Stderr = true
		}
		if entry.Etag != "" && bs.etag != "" && entry.Etag != bs.etag {
			continue
		}
		if entry.Service != "" && len(bs.services) > 0 && !slices.Contains(bs.services, entry.Service) {
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
